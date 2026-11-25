package nanoandroid.runtime;

import java.io.*;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.*;
import java.util.zip.ZipEntry;
import java.util.zip.ZipFile;

/**
 * DexClassLoader ingests classes.dex directly from APKs or raw byte streams
 * into an in-memory class registry on host OpenJDK 25.
 */
public class DexClassLoader {
    public static class DexMethodEntry {
        public final String name;
        public final String returnType;
        public final List<String> parameterTypes;
        public final int accessFlags;

        public DexMethodEntry(String name, String returnType, List<String> parameterTypes, int accessFlags) {
            this.name = name;
            this.returnType = returnType;
            this.parameterTypes = parameterTypes != null ? parameterTypes : Collections.emptyList();
            this.accessFlags = accessFlags;
        }
    }

    public static class DexClassEntry {
        public final String descriptor;     // Lcom/example/MainActivity;
        public final String dottedName;     // com.example.MainActivity
        public final String superClassName;
        public final int accessFlags;
        public final Map<String, DexMethodEntry> methods = new HashMap<>();
        public final Map<String, Object> staticFields = new HashMap<>();

        public DexClassEntry(String descriptor, String superClassName, int accessFlags) {
            this.descriptor = descriptor;
            this.dottedName = toDotted(descriptor);
            this.superClassName = superClassName;
            this.accessFlags = accessFlags;
        }

        public DexMethodEntry findMethod(String name) {
            return methods.get(name);
        }
    }

    private final Map<String, DexClassEntry> classRegistry = new HashMap<>();
    private final Map<String, MethodInvocationHook> methodHooks = new HashMap<>();

    @FunctionalInterface
    public interface MethodInvocationHook {
        Object invoke(Object instance, Object[] args) throws Exception;
    }

    public DexClassLoader() {}

    public synchronized void registerClass(DexClassEntry entry) {
        classRegistry.put(entry.descriptor, entry);
        classRegistry.put(entry.dottedName, entry);
    }

    public synchronized DexClassEntry resolveClass(String name) {
        if (classRegistry.containsKey(name)) {
            return classRegistry.get(name);
        }
        String desc = toDescriptor(name);
        if (classRegistry.containsKey(desc)) {
            return classRegistry.get(desc);
        }
        String dotted = toDotted(name);
        if (classRegistry.containsKey(dotted)) {
            return classRegistry.get(dotted);
        }
        return null;
    }

    public synchronized void registerMethodHook(String className, String methodName, MethodInvocationHook hook) {
        methodHooks.put(toDotted(className) + "#" + methodName, hook);
        methodHooks.put(toDescriptor(className) + "->" + methodName, hook);
    }

    public synchronized Object invokeMethod(DexClassEntry clazz, String methodName, Object instance, Object... args) throws Exception {
        if (clazz == null) {
            throw new IllegalArgumentException("Cannot invoke method on null DexClassEntry");
        }
        String key = clazz.dottedName + "#" + methodName;
        MethodInvocationHook hook = methodHooks.get(key);
        if (hook != null) {
            return hook.invoke(instance, args);
        }
        // Default lifecycle execution returns successfully
        return null;
    }

    public synchronized void loadFromApk(File apkFile) throws IOException {
        try (ZipFile zip = new ZipFile(apkFile)) {
            Enumeration<? extends ZipEntry> entries = zip.entries();
            while (entries.hasMoreElements()) {
                ZipEntry entry = entries.nextElement();
                if (entry.getName().startsWith("classes") && entry.getName().endsWith(".dex")) {
                    try (InputStream is = zip.getInputStream(entry)) {
                        byte[] dexBytes = is.readAllBytes();
                        loadFromDexBytes(dexBytes);
                    }
                }
            }
        }
    }

    public synchronized void loadFromDexBytes(byte[] data) throws IOException {
        if (data.length < 112) {
            throw new IOException("DEX file too small: " + data.length);
        }

        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);
        byte[] magic = new byte[8];
        buf.get(magic);
        if (magic[0] != 'd' || magic[1] != 'e' || magic[2] != 'x' || magic[3] != '\n') {
            throw new IOException("Invalid DEX magic header");
        }

        buf.position(0x38);
        int stringIdsSize = buf.getInt();
        int stringIdsOff = buf.getInt();
        int typeIdsSize = buf.getInt();
        int typeIdsOff = buf.getInt();
        int protoIdsSize = buf.getInt();
        int protoIdsOff = buf.getInt();
        int fieldIdsSize = buf.getInt();
        int fieldIdsOff = buf.getInt();
        int methodIdsSize = buf.getInt();
        int methodIdsOff = buf.getInt();
        int classDefsSize = buf.getInt();
        int classDefsOff = buf.getInt();

        // Read strings
        String[] strings = new String[stringIdsSize];
        for (int i = 0; i < stringIdsSize; i++) {
            buf.position(stringIdsOff + (i * 4));
            int strDataOff = buf.getInt();
            strings[i] = readMutf8(data, strDataOff);
        }

        // Read types
        String[] types = new String[typeIdsSize];
        for (int i = 0; i < typeIdsSize; i++) {
            buf.position(typeIdsOff + (i * 4));
            int descriptorIdx = buf.getInt();
            if (descriptorIdx >= 0 && descriptorIdx < strings.length) {
                types[i] = strings[descriptorIdx];
            }
        }

        // Read class defs
        for (int i = 0; i < classDefsSize; i++) {
            buf.position(classDefsOff + (i * 32));
            int classIdx = buf.getInt();
            int accessFlags = buf.getInt();
            int superclassIdx = buf.getInt();
            buf.getInt(); // interfaces_off
            buf.getInt(); // source_file_idx
            buf.getInt(); // annotations_off
            int classDataOff = buf.getInt();

            if (classIdx >= 0 && classIdx < types.length) {
                String className = types[classIdx];
                String superName = (superclassIdx >= 0 && superclassIdx < types.length) ? types[superclassIdx] : null;
                DexClassEntry entry = new DexClassEntry(className, superName, accessFlags);
                registerClass(entry);
            }
        }
    }

    private static String readMutf8(byte[] data, int offset) {
        // Skip ULEB128 utf16_size
        int cur = offset;
        while (cur < data.length && (data[cur] & 0x80) != 0) {
            cur++;
        }
        cur++; // skip last byte of uleb
        StringBuilder sb = new StringBuilder();
        while (cur < data.length && data[cur] != 0) {
            sb.append((char) data[cur]);
            cur++;
        }
        return sb.toString();
    }

    public static String toDotted(String desc) {
        if (desc.startsWith("L") && desc.endsWith(";")) {
            return desc.substring(1, desc.length() - 1).replace('/', '.');
        }
        return desc;
    }

    public static String toDescriptor(String dotted) {
        if (dotted.startsWith("L") && dotted.endsWith(";")) {
            return dotted;
        }
        return "L" + dotted.replace('.', '/') + ";";
    }
}
