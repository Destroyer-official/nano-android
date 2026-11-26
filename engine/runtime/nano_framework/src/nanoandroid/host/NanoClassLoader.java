package nanoandroid.host;

import org.objectweb.asm.*;
import org.objectweb.asm.tree.*;
import org.objectweb.asm.tree.analysis.*;
import java.io.*;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Array;
import java.lang.reflect.Proxy;
import java.util.*;
import java.util.zip.*;
import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import android.app.Activity;
import android.view.View;
import android.view.ViewGroup;
import sun.misc.Unsafe;

/**
 * NanoClassLoader dynamically loads and instruments real Android APK / JAR
 * bytecode
 * on host OpenJDK 25, injecting forwarding constructors, ViewBinding
 * synthesizers,
 * and intercepting framework hooks for zero-emulator execution.
 */
public class NanoClassLoader extends ClassLoader {
    private final File jarFile;
    private final File apkFile;
    private ZipFile apkZip;
    private final Map<String, byte[]> rawClasses = new HashMap<>();
    private final Map<String, Set<String>> classNeededInits = new HashMap<>();
    private final Map<String, Set<String>> classDeclaredInits = new HashMap<>();
    private final Map<String, String> classSuperMap = new HashMap<>();
    private final Map<String, String[]> classInterfacesMap = new HashMap<>();
    private final Map<String, Integer> classAccessMap = new HashMap<>();
    private final Set<String> classesInstantiatedWithNew = new HashSet<>();

    public static Activity currentActivity;
    public static java.util.function.Consumer<Intent> onStartActivityHook;
    public static java.util.function.BiConsumer<Intent, Integer> onStartActivityForResultHook;
    public static java.util.function.Consumer<Activity> onFinishActivityHook;

    public NanoClassLoader(File jarFile, File apkFile, ClassLoader parent) throws IOException {
        super(parent);
        this.jarFile = jarFile;
        this.apkFile = apkFile;
        if (apkFile != null && apkFile.exists()) {
            this.apkZip = new ZipFile(apkFile);
        }
        if (jarFile != null && jarFile.exists()) {
            try (ZipFile zip = new ZipFile(jarFile)) {
                Enumeration<? extends ZipEntry> entries = zip.entries();
                while (entries.hasMoreElements()) {
                    ZipEntry entry = entries.nextElement();
                    if (entry.getName().endsWith(".class")) {
                        String className = entry.getName().replace('/', '.').substring(0, entry.getName().length() - 6);
                        try (InputStream is = zip.getInputStream(entry)) {
                            ByteArrayOutputStream baos = new ByteArrayOutputStream();
                            byte[] buf = new byte[8192];
                            int r;
                            while ((r = is.read(buf)) != -1) {
                                baos.write(buf, 0, r);
                            }
                            rawClasses.put(className, baos.toByteArray());
                        }
                    }
                }
            }
        }
        preScanClasses();
    }

    @Override
    public InputStream getResourceAsStream(String name) {
        if (apkZip != null) {
            ZipEntry ze = apkZip.getEntry(name);
            if (ze != null) {
                try {
                    return apkZip.getInputStream(ze);
                } catch (IOException ignored) {
                }
            }
        }
        return super.getResourceAsStream(name);
    }

    public static android.view.MenuItem findOrCreateMenuItem(android.view.Menu menu, int id) {
        if (menu == null)
            return null;
        android.view.MenuItem item = menu.findItem(id);
        if (item == null) {
            item = menu.add(0, id, 0, "Item_" + id);
        }
        return item;
    }

    /**
     * Creates a dynamic Proxy implementing {@code iface} for degraded bytecode
     * where
     * R8/dex2jar emitted {@code new Object()} for a removed/inlined interface
     * implementation.
     */
    public static Object createDummyInterfaceProxy(Class<?> iface) {
        if (iface == null || !iface.isInterface())
            return null;
        ClassLoader cl = iface.getClassLoader() != null ? iface.getClassLoader()
                : NanoClassLoader.class.getClassLoader();
        return Proxy.newProxyInstance(cl, new Class<?>[] { iface }, (proxy, method, args) -> {
            String mName = method.getName();
            int paramCount = method.getParameterCount();
            if ("toString".equals(mName) && paramCount == 0) {
                return iface.getName() + "@NanoDummyProxy";
            }
            if ("hashCode".equals(mName) && paramCount == 0) {
                return System.identityHashCode(proxy);
            }
            if ("equals".equals(mName) && paramCount == 1) {
                return proxy == (args != null && args.length > 0 ? args[0] : null);
            }
            Class<?> rt = method.getReturnType();
            if (rt == void.class)
                return null;
            if (rt == boolean.class)
                return Boolean.FALSE;
            if (rt == byte.class)
                return (byte) 0;
            if (rt == short.class)
                return (short) 0;
            if (rt == char.class)
                return (char) 0;
            if (rt == int.class)
                return 0;
            if (rt == long.class)
                return 0L;
            if (rt == float.class)
                return 0.0f;
            if (rt == double.class)
                return 0.0d;
            if (rt == String.class)
                return "";
            return null;
        });
    }

    private void preScanClasses() {
        for (Map.Entry<String, byte[]> entry : rawClasses.entrySet()) {
            String cName = entry.getKey().replace('.', '/');
            try {
                ClassReader cr = new ClassReader(entry.getValue());
                cr.accept(new ClassVisitor(Opcodes.ASM9) {
                    @Override
                    public void visit(int version, int access, String name, String signature, String superName,
                            String[] interfaces) {
                        classAccessMap.put(name, access);
                        if (superName != null) {
                            classSuperMap.put(name, superName);
                        }
                        if (interfaces != null && interfaces.length > 0) {
                            classInterfacesMap.put(name, interfaces);
                        }
                        super.visit(version, access, name, signature, superName, interfaces);
                    }

                    @Override
                    public MethodVisitor visitMethod(int access, String name, String descriptor, String signature,
                            String[] exceptions) {
                        if ("<init>".equals(name)) {
                            classDeclaredInits.computeIfAbsent(cName, k -> new HashSet<>()).add(descriptor);
                        }
                        return new MethodVisitor(Opcodes.ASM9) {
                            @Override
                            public void visitTypeInsn(int opcode, String type) {
                                if (opcode == Opcodes.NEW && type != null) {
                                    classesInstantiatedWithNew.add(type);
                                }
                                super.visitTypeInsn(opcode, type);
                            }

                            @Override
                            public void visitFieldInsn(int opcode, String owner, String fName, String fDesc) {
                                if ((opcode == Opcodes.PUTSTATIC || opcode == Opcodes.PUTFIELD)
                                        && fDesc != null && fDesc.startsWith("L") && fDesc.endsWith(";")) {
                                    String t = fDesc.substring(1, fDesc.length() - 1);
                                    if (!"java/lang/Object".equals(t) && rawClasses.containsKey(t.replace('/', '.'))) {
                                        classNeededInits.computeIfAbsent(t, k -> new HashSet<>()).add("()V");
                                    }
                                }
                                super.visitFieldInsn(opcode, owner, fName, fDesc);
                            }

                            @Override
                            public void visitMethodInsn(int opcode, String owner, String mName, String descriptor,
                                    boolean isInterface) {
                                if (opcode == Opcodes.INVOKESPECIAL && "<init>".equals(mName)) {
                                    classNeededInits.computeIfAbsent(owner, k -> new HashSet<>()).add(descriptor);
                                }
                                super.visitMethodInsn(opcode, owner, mName, descriptor, isInterface);
                            }
                        };
                    }
                }, ClassReader.SKIP_DEBUG | ClassReader.SKIP_FRAMES);
            } catch (Throwable ignored) {
            }
        }
    }

    public static Unsafe getUnsafe() {
        try {
            Field f = Unsafe.class.getDeclaredField("theUnsafe");
            f.setAccessible(true);
            return (Unsafe) f.get(null);
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    public static void initViewDefaults(View v, Context ctx) {
        if (v == null)
            return;
        try {
            v.setContext(ctx);
            v.setEnabled(true);
            v.setAlpha(1.0f);
            v.setScaleX(1.0f);
            v.setScaleY(1.0f);
            v.setVisibility(View.VISIBLE);
            if (v instanceof android.widget.TextView) {
                android.widget.TextView tv = (android.widget.TextView) v;
                if (tv.getTextSize() <= 0f)
                    tv.setTextSize(14f);
                if (tv.getTypeface() == null)
                    tv.setTypeface(android.graphics.Typeface.DEFAULT);
                tv.setTextColor(0xFF1E293B);
            }
            if (v instanceof android.widget.ProgressBar) {
                android.widget.ProgressBar pb = (android.widget.ProgressBar) v;
                if (pb.getMax() <= 0)
                    pb.setMax(100);
            }
            // Populate null collection/binding/paint/rect fields left zeroed by
            // Unsafe.allocateInstance
            Class<?> cur = v.getClass();
            while (cur != null && cur != View.class && cur != Object.class
                    && !cur.getName().startsWith("android.widget.")) {
                for (Field f : cur.getDeclaredFields()) {
                    try {
                        f.setAccessible(true);
                        if (f.get(v) != null)
                            continue;
                        Class<?> ft = f.getType();
                        if (ft == java.util.ArrayList.class || ft == java.util.List.class) {
                            f.set(v, new java.util.ArrayList<>());
                        } else if (ft == java.util.HashMap.class || ft == java.util.Map.class) {
                            f.set(v, new java.util.HashMap<>());
                        } else if (ft == java.util.HashSet.class || ft == java.util.Set.class) {
                            f.set(v, new java.util.HashSet<>());
                        } else if (ft == android.graphics.Rect.class) {
                            f.set(v, new android.graphics.Rect());
                        } else if (ft == android.graphics.RectF.class) {
                            f.set(v, new android.graphics.RectF());
                        } else if (ft == android.graphics.Paint.class) {
                            f.set(v, new android.graphics.Paint());
                        } else if (ft == android.graphics.Path.class) {
                            f.set(v, new android.graphics.Path());
                        }
                    } catch (Throwable ignored2) {
                    }
                }
                cur = cur.getSuperclass();
            }
        } catch (Throwable ignored) {
        }
    }

    public static boolean isViewBinding(Class<?> clazz) {
        if (clazz == null || clazz == Object.class)
            return false;
        if (clazz.getName().endsWith("Binding"))
            return true;
        for (Class<?> iface : clazz.getInterfaces()) {
            if ("v4.a".equals(iface.getName()) || "androidx.viewbinding.ViewBinding".equals(iface.getName())) {
                return true;
            }
        }
        return false;
    }

    public static void populateFields(Object obj, Context context) {
        populateFieldsInternal(obj, context, 0, Collections.newSetFromMap(new IdentityHashMap<>()));
    }

    private static void populateFieldsInternal(Object obj, Context context, int depth, Set<Object> visited) {
        if (obj == null || depth > 2 || !visited.add(obj))
            return;
        Class<?> current = obj.getClass();
        if (current.getName().startsWith("android.view.") || current.getName().startsWith("android.graphics."))
            return;
        while (current != null && current != Object.class && !current.getName().startsWith("java.")) {
            for (Field field : current.getDeclaredFields()) {
                field.setAccessible(true);
                try {
                    Class<?> fType = field.getType();
                    boolean isVB = isViewBinding(fType);
                    if (isVB) {
                        if (field.get(obj) == null) {
                            Object subBinding = instantiateViewBinding(fType, context);
                            field.set(obj, subBinding);
                        }
                    } else if (View.class.isAssignableFrom(fType) && field.get(obj) == null) {
                        try {
                            View childView = null;
                            try {
                                childView = (View) fType.getConstructor(Context.class).newInstance(context);
                            } catch (Throwable t1) {
                                try {
                                    childView = (View) fType
                                            .getConstructor(Context.class, android.util.AttributeSet.class)
                                            .newInstance(context, android.util.Xml.asAttributeSet(
                                                    new android.content.res.HeadlessXmlResourceParser()));
                                } catch (Throwable t2) {
                                    Unsafe unsafe = getUnsafe();
                                    childView = (View) unsafe.allocateInstance(fType);
                                    childView.setUseSafeLayoutFallback(true);
                                    initViewDefaults(childView, context);
                                }
                            }
                            if (childView != null) {
                                field.set(obj, childView);
                                populateFieldsInternal(childView, context, depth + 1, visited);
                            }
                        } catch (Throwable ignored) {
                        }
                    } else if (android.graphics.drawable.Drawable.class.isAssignableFrom(fType)
                            && field.get(obj) == null) {
                        try {
                            android.graphics.drawable.Drawable d = null;
                            try {
                                d = (android.graphics.drawable.Drawable) fType.getConstructor(Context.class)
                                        .newInstance(context);
                            } catch (Throwable t1) {
                                try {
                                    d = (android.graphics.drawable.Drawable) fType.getDeclaredConstructor()
                                            .newInstance();
                                } catch (Throwable t2) {
                                    Unsafe unsafe = getUnsafe();
                                    d = (android.graphics.drawable.Drawable) unsafe.allocateInstance(fType);
                                }
                            }
                            if (d != null) {
                                field.set(obj, d);
                                populateFieldsInternal(d, context, depth + 1, visited);
                            }
                        } catch (Throwable ignored) {
                        }
                    } else if (fType.isArray() && field.get(obj) == null) {
                        try {
                            if (fType == int[].class) {
                                field.set(obj, new int[] { 0xFF000000, 0xFF4488EE });
                            } else if (fType == float[].class) {
                                field.set(obj, new float[] { 0f, 1f });
                            } else if (fType == String[].class) {
                                field.set(obj, new String[] { "" });
                            } else {
                                field.set(obj, Array.newInstance(fType.getComponentType(), 0));
                            }
                        } catch (Throwable ignored) {
                        }
                    } else if (android.animation.Animator.class.isAssignableFrom(fType) && field.get(obj) == null) {
                        try {
                            field.set(obj, new android.animation.ValueAnimator());
                        } catch (Throwable ignored) {
                        }
                    }
                } catch (Throwable ignored) {
                }
            }
            current = current.getSuperclass();
        }
    }

    public static Object instantiateViewBinding(Class<?> bindingClass, Context context) {
        try {
            // Try real generic ViewBinding.inflate(LayoutInflater) first if present on
            // compiled binding class
            try {
                Method inflateM = bindingClass.getMethod("inflate", android.view.LayoutInflater.class);
                return inflateM.invoke(null, android.view.LayoutInflater.from(context));
            } catch (Throwable ignored) {
            }

            Unsafe unsafe = getUnsafe();
            Object binding = unsafe.allocateInstance(bindingClass);
            ViewGroup root = null;

            for (Field field : bindingClass.getDeclaredFields()) {
                field.setAccessible(true);
                Class<?> fType = field.getType();
                if (View.class.isAssignableFrom(fType)) {
                    View v = null;
                    try {
                        Constructor<?> c = fType.getConstructor(Context.class);
                        v = (View) c.newInstance(context);
                    } catch (Throwable t1) {
                        try {
                            Constructor<?> c = fType.getConstructor(Context.class, android.util.AttributeSet.class);
                            v = (View) c.newInstance(context, android.util.Xml
                                    .asAttributeSet(new android.content.res.HeadlessXmlResourceParser()));
                        } catch (Throwable t2) {
                            try {
                                v = (View) unsafe.allocateInstance(fType);
                                v.setUseSafeLayoutFallback(true);
                                initViewDefaults(v, context);
                            } catch (Throwable ignored) {
                            }
                        }
                    }

                    if (v != null) {
                        try {
                            Field ctxF = View.class.getDeclaredField("context");
                            ctxF.setAccessible(true);
                            if (ctxF.get(v) == null) {
                                ctxF.set(v, context);
                            }
                        } catch (Throwable ignored) {
                        }

                        if (v.getLayoutParams() == null) {
                            v.setLayoutParams(new ViewGroup.MarginLayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,
                                    ViewGroup.LayoutParams.WRAP_CONTENT));
                        }

                        populateFields(v, context);
                        field.set(binding, v);
                        if (root == null && v instanceof ViewGroup) {
                            root = (ViewGroup) v;
                        } else if (root != null) {
                            root.addView(v);
                        }
                    }
                } else if (isViewBinding(fType)) {
                    Object subBinding = instantiateViewBinding(fType, context);
                    field.set(binding, subBinding);
                }
            }
            populateFields(binding, context);

            if (currentActivity != null && root != null) {
                currentActivity.setContentView(root);
            }

            return binding;
        } catch (Throwable t) {
            return null;
        }
    }

    @Override
    protected Class<?> findClass(String name) throws ClassNotFoundException {
        byte[] bytes = rawClasses.get(name);
        if (bytes == null) {
            return super.findClass(name);
        }

        byte[] transformed = transformClass(name, bytes);
        return defineClass(name, transformed, 0, transformed.length);
    }

    private static AbstractInsnNode nextReal(AbstractInsnNode insn) {
        AbstractInsnNode cur = insn != null ? insn.getNext() : null;
        while (cur != null && (cur instanceof LabelNode || cur instanceof LineNumberNode || cur instanceof FrameNode)) {
            cur = cur.getNext();
        }
        return cur;
    }

    private static boolean hasNoLabelsOrInsnsBetween(AbstractInsnNode a, AbstractInsnNode b) {
        if (a == null || b == null)
            return false;
        AbstractInsnNode cur = a.getNext();
        while (cur != null && cur != b) {
            if (cur instanceof LabelNode)
                return false;
            if (!(cur instanceof LineNumberNode || cur instanceof FrameNode))
                return false;
            cur = cur.getNext();
        }
        return cur == b;
    }

    private static int[] stackEffectValues(AbstractInsnNode insn) {
        if (insn == null || insn instanceof LabelNode || insn instanceof LineNumberNode || insn instanceof FrameNode) {
            return new int[] { 0, 0 };
        }
        int op = insn.getOpcode();
        if (insn instanceof InsnNode) {
            switch (op) {
                case Opcodes.NOP:
                case Opcodes.RETURN:
                    return new int[] { 0, 0 };
                case Opcodes.ACONST_NULL:
                case Opcodes.ICONST_M1:
                case Opcodes.ICONST_0:
                case Opcodes.ICONST_1:
                case Opcodes.ICONST_2:
                case Opcodes.ICONST_3:
                case Opcodes.ICONST_4:
                case Opcodes.ICONST_5:
                case Opcodes.LCONST_0:
                case Opcodes.LCONST_1:
                case Opcodes.FCONST_0:
                case Opcodes.FCONST_1:
                case Opcodes.FCONST_2:
                case Opcodes.DCONST_0:
                case Opcodes.DCONST_1:
                    return new int[] { 0, 1 };
                case Opcodes.IALOAD:
                case Opcodes.LALOAD:
                case Opcodes.FALOAD:
                case Opcodes.DALOAD:
                case Opcodes.AALOAD:
                case Opcodes.BALOAD:
                case Opcodes.CALOAD:
                case Opcodes.SALOAD:
                    return new int[] { 2, 1 };
                case Opcodes.IASTORE:
                case Opcodes.LASTORE:
                case Opcodes.FASTORE:
                case Opcodes.DASTORE:
                case Opcodes.AASTORE:
                case Opcodes.BASTORE:
                case Opcodes.CASTORE:
                case Opcodes.SASTORE:
                    return new int[] { 3, 0 };
                case Opcodes.POP:
                    return new int[] { 1, 0 };
                case Opcodes.DUP:
                    return new int[] { 0, 1 };
                case Opcodes.SWAP:
                    return new int[] { 2, 2 };
                case Opcodes.IADD:
                case Opcodes.LADD:
                case Opcodes.FADD:
                case Opcodes.DADD:
                case Opcodes.ISUB:
                case Opcodes.LSUB:
                case Opcodes.FSUB:
                case Opcodes.DSUB:
                case Opcodes.IMUL:
                case Opcodes.LMUL:
                case Opcodes.FMUL:
                case Opcodes.DMUL:
                case Opcodes.IDIV:
                case Opcodes.LDIV:
                case Opcodes.FDIV:
                case Opcodes.DDIV:
                case Opcodes.IREM:
                case Opcodes.LREM:
                case Opcodes.FREM:
                case Opcodes.DREM:
                case Opcodes.ISHL:
                case Opcodes.LSHL:
                case Opcodes.ISHR:
                case Opcodes.LSHR:
                case Opcodes.IUSHR:
                case Opcodes.LUSHR:
                case Opcodes.IAND:
                case Opcodes.LAND:
                case Opcodes.IOR:
                case Opcodes.LOR:
                case Opcodes.IXOR:
                case Opcodes.LXOR:
                case Opcodes.LCMP:
                case Opcodes.FCMPL:
                case Opcodes.FCMPG:
                case Opcodes.DCMPL:
                case Opcodes.DCMPG:
                    return new int[] { 2, 1 };
                case Opcodes.INEG:
                case Opcodes.LNEG:
                case Opcodes.FNEG:
                case Opcodes.DNEG:
                case Opcodes.I2L:
                case Opcodes.I2F:
                case Opcodes.I2D:
                case Opcodes.L2I:
                case Opcodes.L2F:
                case Opcodes.L2D:
                case Opcodes.F2I:
                case Opcodes.F2L:
                case Opcodes.F2D:
                case Opcodes.D2I:
                case Opcodes.D2L:
                case Opcodes.D2F:
                case Opcodes.I2B:
                case Opcodes.I2C:
                case Opcodes.I2S:
                case Opcodes.ARRAYLENGTH:
                    return new int[] { 1, 1 };
                case Opcodes.IRETURN:
                case Opcodes.LRETURN:
                case Opcodes.FRETURN:
                case Opcodes.DRETURN:
                case Opcodes.ARETURN:
                case Opcodes.ATHROW:
                case Opcodes.MONITORENTER:
                case Opcodes.MONITOREXIT:
                    return new int[] { 1, 0 };
                default:
                    return null;
            }
        }
        if (insn instanceof IntInsnNode) {
            return op == Opcodes.NEWARRAY ? new int[] { 1, 1 } : new int[] { 0, 1 };
        }
        if (insn instanceof LdcInsnNode) {
            return new int[] { 0, 1 };
        }
        if (insn instanceof VarInsnNode) {
            if (op >= Opcodes.ILOAD && op <= Opcodes.ALOAD)
                return new int[] { 0, 1 };
            if (op >= Opcodes.ISTORE && op <= Opcodes.ASTORE)
                return new int[] { 1, 0 };
            return null;
        }
        if (insn instanceof IincInsnNode) {
            return new int[] { 0, 0 };
        }
        if (insn instanceof FieldInsnNode) {
            switch (op) {
                case Opcodes.GETSTATIC:
                    return new int[] { 0, 1 };
                case Opcodes.PUTSTATIC:
                    return new int[] { 1, 0 };
                case Opcodes.GETFIELD:
                    return new int[] { 1, 1 };
                case Opcodes.PUTFIELD:
                    return new int[] { 2, 0 };
            }
        }
        if (insn instanceof MethodInsnNode) {
            MethodInsnNode min = (MethodInsnNode) insn;
            int pops = org.objectweb.asm.Type.getArgumentTypes(min.desc).length + (op == Opcodes.INVOKESTATIC ? 0 : 1);
            int pushes = org.objectweb.asm.Type.getReturnType(min.desc).getSort() == org.objectweb.asm.Type.VOID ? 0
                    : 1;
            return new int[] { pops, pushes };
        }
        if (insn instanceof InvokeDynamicInsnNode) {
            InvokeDynamicInsnNode indy = (InvokeDynamicInsnNode) insn;
            int pops = org.objectweb.asm.Type.getArgumentTypes(indy.desc).length;
            int pushes = org.objectweb.asm.Type.getReturnType(indy.desc).getSort() == org.objectweb.asm.Type.VOID ? 0
                    : 1;
            return new int[] { pops, pushes };
        }
        if (insn instanceof TypeInsnNode) {
            switch (op) {
                case Opcodes.NEW:
                    return new int[] { 0, 1 };
                case Opcodes.ANEWARRAY:
                case Opcodes.CHECKCAST:
                case Opcodes.INSTANCEOF:
                    return new int[] { 1, 1 };
            }
        }
        if (insn instanceof MultiANewArrayInsnNode) {
            return new int[] { ((MultiANewArrayInsnNode) insn).dims, 1 };
        }
        return null;
    }

    private int getRawClassAccess(String internalName) {
        if (internalName == null)
            return 0;
        Integer acc = classAccessMap.get(internalName);
        if (acc != null)
            return acc;
        byte[] b = rawClasses.get(internalName.replace('/', '.'));
        if (b != null) {
            try {
                int a = new ClassReader(b).getAccess();
                classAccessMap.put(internalName, a);
                return a;
            } catch (Throwable ignored) {
            }
        }
        return 0;
    }

    private boolean isConcreteRawClass(String internalName) {
        if (internalName == null || !rawClasses.containsKey(internalName.replace('/', '.')))
            return false;
        return (getRawClassAccess(internalName) & (Opcodes.ACC_INTERFACE | Opcodes.ACC_ABSTRACT)) == 0;
    }

    private boolean isRawInterface(String internalName) {
        if (internalName == null)
            return false;
        if (rawClasses.containsKey(internalName.replace('/', '.'))) {
            return (getRawClassAccess(internalName) & Opcodes.ACC_INTERFACE) != 0;
        }
        try {
            return Class.forName(internalName.replace('/', '.'), false, getParent()).isInterface();
        } catch (Throwable ignored) {
            return false;
        }
    }

    private boolean canProvideZeroArgInit(String internalName) {
        if (isRawInterface(internalName))
            return false;
        String cur = internalName;
        for (int hop = 0; hop < 12 && cur != null; hop++) {
            if ("java/lang/Object".equals(cur))
                return true;
            if ("java/lang/Enum".equals(cur))
                return false;
            if (isRawInterface(cur))
                return false;
            if (rawClasses.containsKey(cur.replace('/', '.'))) {
                cur = classSuperMap.get(cur);
                continue;
            }
            try {
                Class<?> c = getParent().loadClass(cur.replace('/', '.'));
                if (c.isInterface())
                    return false;
                c.getDeclaredConstructor();
                return true;
            } catch (Throwable t) {
                return false;
            }
        }
        return false;
    }

    private boolean hasSuperInit(String superInternal, String desc) {
        String cur = superInternal;
        for (int hop = 0; hop < 16 && cur != null; hop++) {
            if ("java/lang/Object".equals(cur))
                return "()V".equals(desc);
            if ("java/lang/Enum".equals(cur))
                return "(Ljava/lang/String;I)V".equals(desc);
            Set<String> declared = classDeclaredInits.get(cur);
            if (declared != null && declared.contains(desc))
                return true;
            Set<String> needed = classNeededInits.get(cur);
            if (needed != null && needed.contains(desc))
                return true;
            if (rawClasses.containsKey(cur.replace('/', '.'))) {
                if (declared == null || declared.isEmpty()) {
                    if ("()V".equals(desc) && canProvideZeroArgInit(cur))
                        return true;
                    cur = classSuperMap.get(cur);
                    continue;
                }
                return "()V".equals(desc) && canProvideZeroArgInit(cur);
            }
            try {
                Class<?> c = getParent().loadClass(cur.replace('/', '.'));
                for (Constructor<?> ctor : c.getDeclaredConstructors()) {
                    if (org.objectweb.asm.Type.getConstructorDescriptor(ctor).equals(desc))
                        return true;
                }
            } catch (Throwable ignored) {
            }
            return false;
        }
        return false;
    }

    private boolean canSatisfySubClassInitChain(String subInternal, String ancestorInternal, String ctorDesc,
            List<String> chainOut) {
        if (subInternal == null || ancestorInternal == null || isRawInterface(subInternal))
            return false;
        List<String> chain = new ArrayList<>();
        String cur = subInternal;
        for (int hop = 0; hop < 16 && cur != null; hop++) {
            if (cur.equals(ancestorInternal)) {
                if ("java/lang/Object".equals(ancestorInternal) && !"()V".equals(ctorDesc))
                    return false;
                if (!"java/lang/Object".equals(ancestorInternal) && !hasSuperInit(ancestorInternal, ctorDesc)
                        && !(classDeclaredInits.getOrDefault(ancestorInternal, Collections.emptySet())
                                .contains(ctorDesc))) {
                    boolean hasAnc = "()V".equals(ctorDesc) && canProvideZeroArgInit(ancestorInternal);
                    if (!hasAnc)
                        return false;
                }
                if (chainOut != null) {
                    chainOut.addAll(chain);
                }
                return true;
            }
            if (!rawClasses.containsKey(cur.replace('/', '.')) || isRawInterface(cur)) {
                return false;
            }
            chain.add(cur);
            cur = classSuperMap.get(cur);
        }
        return false;
    }

    private boolean ensureSubClassInitChain(String subInternal, String ancestorInternal, String ctorDesc) {
        List<String> chain = new ArrayList<>();
        if (!canSatisfySubClassInitChain(subInternal, ancestorInternal, ctorDesc, chain)) {
            return false;
        }
        for (String cls : chain) {
            classNeededInits.computeIfAbsent(cls, k -> new HashSet<>()).add(ctorDesc);
        }
        return true;
    }

    private String resolveCommonSuper(String type1, String type2) {
        if (type1 == null || type2 == null)
            return "java/lang/Object";
        if (type1.equals(type2))
            return type1;
        Set<String> chain1 = new HashSet<>();
        String cur = type1;
        for (int i = 0; i < 20 && cur != null; i++) {
            chain1.add(cur);
            if ("java/lang/Object".equals(cur))
                break;
            cur = getSuperOf(cur);
        }
        cur = type2;
        for (int i = 0; i < 20 && cur != null; i++) {
            if (chain1.contains(cur))
                return cur;
            if ("java/lang/Object".equals(cur))
                break;
            cur = getSuperOf(cur);
        }
        return "java/lang/Object";
    }

    private String getSuperOf(String internalName) {
        String s = classSuperMap.get(internalName);
        if (s != null)
            return s;
        try {
            Class<?> c = Class.forName(internalName.replace('/', '.'), false, getParent());
            Class<?> sup = c.getSuperclass();
            return sup != null ? sup.getName().replace('.', '/') : "java/lang/Object";
        } catch (Throwable t) {
            return "java/lang/Object";
        }
    }

    private static String extractConsumerTarget(MethodNode mn, AbstractInsnNode p, int depth) {
        if (p instanceof FieldInsnNode) {
            FieldInsnNode fn = (FieldInsnNode) p;
            if (p.getOpcode() == Opcodes.GETFIELD)
                return fn.owner;
            if (p.getOpcode() == Opcodes.PUTFIELD) {
                if (depth == 1)
                    return fn.owner;
                if (depth == 0 && fn.desc.startsWith("L") && fn.desc.endsWith(";")) {
                    return fn.desc.substring(1, fn.desc.length() - 1);
                }
            }
            if (p.getOpcode() == Opcodes.PUTSTATIC && fn.desc.startsWith("L") && fn.desc.endsWith(";")) {
                return fn.desc.substring(1, fn.desc.length() - 1);
            }
        } else if (p instanceof MethodInsnNode) {
            MethodInsnNode min = (MethodInsnNode) p;
            org.objectweb.asm.Type[] args = org.objectweb.asm.Type.getArgumentTypes(min.desc);
            boolean hasRecv = p.getOpcode() != Opcodes.INVOKESTATIC;
            if (hasRecv && depth == args.length) {
                if (!"java/lang/Object".equals(min.owner))
                    return min.owner;
            } else if (depth < args.length) {
                int argIdx = args.length - 1 - depth;
                if (args[argIdx].getSort() == org.objectweb.asm.Type.OBJECT) {
                    String argCls = args[argIdx].getInternalName();
                    if (!"java/lang/Object".equals(argCls))
                        return argCls;
                }
            }
        } else if (p instanceof TypeInsnNode && p.getOpcode() == Opcodes.CHECKCAST) {
            String d = ((TypeInsnNode) p).desc;
            if (d != null && !d.startsWith("[") && !"java/lang/Object".equals(d))
                return d;
        } else if (p.getOpcode() == Opcodes.ARETURN) {
            org.objectweb.asm.Type ret = org.objectweb.asm.Type.getReturnType(mn.desc);
            if (ret.getSort() == org.objectweb.asm.Type.OBJECT) {
                String rCls = ret.getInternalName();
                if (!"java/lang/Object".equals(rCls))
                    return rCls;
            }
        }
        return null;
    }

    private static String findTargetOwnerForStackTop(MethodNode mn, AbstractInsnNode start) {
        int depth = 0;
        AbstractInsnNode p = start;
        for (int walk = 0; p != null && walk < 40; walk++, p = p.getNext()) {
            if (p instanceof LabelNode || p instanceof LineNumberNode || p instanceof FrameNode)
                continue;
            if (p instanceof JumpInsnNode || p instanceof TableSwitchInsnNode || p instanceof LookupSwitchInsnNode)
                break;
            int[] eff = stackEffectValues(p);
            if (eff == null)
                break;
            int pops = eff[0];
            int pushes = eff[1];
            if (pops > depth) {
                return extractConsumerTarget(mn, p, depth);
            }
            depth = depth - pops + pushes;
            if (depth < 0)
                break;
        }
        return null;
    }

    private String findTargetOwnerForNewObjectVar(MethodNode mn, AbstractInsnNode astoreNode, int varIdx) {
        AbstractInsnNode cur = astoreNode.getNext();
        Set<AbstractInsnNode> visited = Collections.newSetFromMap(new IdentityHashMap<>());
        for (int steps = 0; cur != null && steps < 120 && visited.add(cur); steps++) {
            if (cur instanceof JumpInsnNode && cur.getOpcode() == Opcodes.GOTO) {
                cur = ((JumpInsnNode) cur).label;
                continue;
            }
            if (cur instanceof VarInsnNode && cur.getOpcode() == Opcodes.ASTORE && ((VarInsnNode) cur).var == varIdx) {
                break;
            }
            if (cur instanceof VarInsnNode && cur.getOpcode() == Opcodes.ALOAD && ((VarInsnNode) cur).var == varIdx) {
                String found = findTargetOwnerForStackTop(mn, cur.getNext());
                if (found != null)
                    return found;
            }
            cur = cur.getNext();
        }
        return null;
    }

    private boolean isSubclassOf(String subInternal, String superInternal) {
        if (subInternal == null || superInternal == null)
            return false;
        if ("java/lang/Object".equals(superInternal) || subInternal.equals(superInternal))
            return true;
        Set<String> seen = new HashSet<>();
        Deque<String> q = new ArrayDeque<>();
        q.add(subInternal);
        while (!q.isEmpty() && seen.size() < 64) {
            String cur = q.poll();
            if (cur == null || !seen.add(cur))
                continue;
            if (superInternal.equals(cur))
                return true;
            String sup = classSuperMap.get(cur);
            if (sup != null && !"java/lang/Object".equals(sup))
                q.add(sup);
            String[] ifaces = classInterfacesMap.get(cur);
            if (ifaces != null) {
                for (String iface : ifaces)
                    q.add(iface);
            }
            if (!rawClasses.containsKey(cur.replace('/', '.'))) {
                try {
                    Class<?> subCls = Class.forName(cur.replace('/', '.'), false, getParent());
                    Class<?> supCls = Class.forName(superInternal.replace('/', '.'), false, getParent());
                    if (supCls.isAssignableFrom(subCls))
                        return true;
                } catch (Throwable ignored) {
                }
            }
        }
        return false;
    }

    private void insertCheckcastAtStackOffset(MethodNode mn, AbstractInsnNode beforeInsn, int valueDepthFromTop,
            String targetInternal, Frame<BasicValue> frame) {
        if ("java/lang/Object".equals(targetInternal) || valueDepthFromTop < 0)
            return;
        int sp = frame.getStackSize();
        if (valueDepthFromTop >= sp)
            return;
        if (valueDepthFromTop == 0) {
            mn.instructions.insertBefore(beforeInsn, new TypeInsnNode(Opcodes.CHECKCAST, targetInternal));
            return;
        }
        List<Integer> temps = new ArrayList<>();
        List<Integer> stores = new ArrayList<>();
        List<Integer> loads = new ArrayList<>();
        for (int k = 0; k < valueDepthFromTop; k++) {
            BasicValue bv = frame.getStack(sp - 1 - k);
            if (bv == null || bv == BasicValue.UNINITIALIZED_VALUE)
                return;
            int sz = bv.getSize();
            org.objectweb.asm.Type t = bv.getType();
            int sort = (t != null) ? t.getSort() : org.objectweb.asm.Type.OBJECT;
            int loc = mn.maxLocals;
            mn.maxLocals += sz;
            temps.add(loc);
            switch (sort) {
                case org.objectweb.asm.Type.BOOLEAN:
                case org.objectweb.asm.Type.BYTE:
                case org.objectweb.asm.Type.CHAR:
                case org.objectweb.asm.Type.SHORT:
                case org.objectweb.asm.Type.INT:
                    stores.add(Opcodes.ISTORE);
                    loads.add(Opcodes.ILOAD);
                    break;
                case org.objectweb.asm.Type.FLOAT:
                    stores.add(Opcodes.FSTORE);
                    loads.add(Opcodes.FLOAD);
                    break;
                case org.objectweb.asm.Type.LONG:
                    stores.add(Opcodes.LSTORE);
                    loads.add(Opcodes.LLOAD);
                    break;
                case org.objectweb.asm.Type.DOUBLE:
                    stores.add(Opcodes.DSTORE);
                    loads.add(Opcodes.DLOAD);
                    break;
                default:
                    stores.add(Opcodes.ASTORE);
                    loads.add(Opcodes.ALOAD);
                    break;
            }
        }
        for (int k = 0; k < temps.size(); k++) {
            mn.instructions.insertBefore(beforeInsn, new VarInsnNode(stores.get(k), temps.get(k)));
        }
        mn.instructions.insertBefore(beforeInsn, new TypeInsnNode(Opcodes.CHECKCAST, targetInternal));
        for (int k = temps.size() - 1; k >= 0; k--) {
            mn.instructions.insertBefore(beforeInsn, new VarInsnNode(loads.get(k), temps.get(k)));
        }
    }

    private boolean shouldCastTo(BasicValue bv, String want, String selfInternal, String methodName) {
        if (bv == null || bv == BasicValue.UNINITIALIZED_VALUE || bv.getType() == null)
            return false;
        org.objectweb.asm.Type t = bv.getType();
        if (t.getSort() != org.objectweb.asm.Type.OBJECT && t.getSort() != org.objectweb.asm.Type.ARRAY)
            return false;
        String tName = t.getInternalName();
        if ("null".equals(tName) || "java/lang/Object".equals(want))
            return false;
        if ("<init>".equals(methodName) && tName.equals(selfInternal))
            return false;
        return !isSubclassOf(tName, want);
    }

    private static List<Integer> findReachingStores(AbstractInsnNode[] insns, int targetIdx, int varIdx) {
        if (targetIdx <= 0 || insns == null || targetIdx >= insns.length)
            return Collections.emptyList();
        for (int k = targetIdx - 1; k >= 0; k--) {
            AbstractInsnNode ins = insns[k];
            if (ins instanceof LabelNode)
                break;
            if (ins instanceof VarInsnNode && ins.getOpcode() == Opcodes.ASTORE && ((VarInsnNode) ins).var == varIdx) {
                return Collections.singletonList(k);
            }
        }
        Map<LabelNode, Integer> labelIdx = new HashMap<>();
        for (int i = 0; i < insns.length; i++) {
            if (insns[i] instanceof LabelNode)
                labelIdx.put((LabelNode) insns[i], i);
        }
        List<List<Integer>> preds = new ArrayList<>(insns.length);
        for (int i = 0; i < insns.length; i++)
            preds.add(null);
        for (int i = 0; i < insns.length; i++) {
            AbstractInsnNode ins = insns[i];
            int op = ins.getOpcode();
            if (ins instanceof JumpInsnNode) {
                Integer tgt = labelIdx.get(((JumpInsnNode) ins).label);
                if (tgt != null) {
                    if (preds.get(tgt) == null)
                        preds.set(tgt, new ArrayList<>(2));
                    preds.get(tgt).add(i);
                }
                if (op != Opcodes.GOTO && i + 1 < insns.length) {
                    if (preds.get(i + 1) == null)
                        preds.set(i + 1, new ArrayList<>(2));
                    preds.get(i + 1).add(i);
                }
            } else if (ins instanceof TableSwitchInsnNode) {
                TableSwitchInsnNode ts = (TableSwitchInsnNode) ins;
                Integer dflt = labelIdx.get(ts.dflt);
                if (dflt != null) {
                    if (preds.get(dflt) == null)
                        preds.set(dflt, new ArrayList<>(2));
                    preds.get(dflt).add(i);
                }
                for (LabelNode l : ts.labels) {
                    Integer tgt = labelIdx.get(l);
                    if (tgt != null) {
                        if (preds.get(tgt) == null)
                            preds.set(tgt, new ArrayList<>(2));
                        preds.get(tgt).add(i);
                    }
                }
            } else if (ins instanceof LookupSwitchInsnNode) {
                LookupSwitchInsnNode ls = (LookupSwitchInsnNode) ins;
                Integer dflt = labelIdx.get(ls.dflt);
                if (dflt != null) {
                    if (preds.get(dflt) == null)
                        preds.set(dflt, new ArrayList<>(2));
                    preds.get(dflt).add(i);
                }
                for (LabelNode l : ls.labels) {
                    Integer tgt = labelIdx.get(l);
                    if (tgt != null) {
                        if (preds.get(tgt) == null)
                            preds.set(tgt, new ArrayList<>(2));
                        preds.get(tgt).add(i);
                    }
                }
            } else if (op != Opcodes.RETURN && op != Opcodes.IRETURN && op != Opcodes.LRETURN
                    && op != Opcodes.FRETURN && op != Opcodes.DRETURN && op != Opcodes.ARETURN
                    && op != Opcodes.ATHROW && i + 1 < insns.length) {
                if (preds.get(i + 1) == null)
                    preds.set(i + 1, new ArrayList<>(2));
                preds.get(i + 1).add(i);
            }
        }
        List<Integer> reaching = new ArrayList<>();
        boolean[] visited = new boolean[insns.length];
        Deque<Integer> q = new ArrayDeque<>();
        List<Integer> startPreds = preds.get(targetIdx);
        if (startPreds != null) {
            for (int p : startPreds) {
                if (!visited[p]) {
                    visited[p] = true;
                    q.add(p);
                }
            }
        }
        while (!q.isEmpty()) {
            int cur = q.poll();
            if (insns[cur] instanceof VarInsnNode && insns[cur].getOpcode() == Opcodes.ASTORE
                    && ((VarInsnNode) insns[cur]).var == varIdx) {
                reaching.add(cur);
                continue;
            }
            List<Integer> ps = preds.get(cur);
            if (ps != null) {
                for (int p : ps) {
                    if (!visited[p]) {
                        visited[p] = true;
                        q.add(p);
                    }
                }
            }
        }
        return reaching;
    }

    private static TypeInsnNode traceSingleNewAllocation(
            SourceValue sv,
            Frame<SourceValue>[] srcFrames,
            AbstractInsnNode[] insns,
            String expectedNewInternal) {
        if (sv == null || sv.insns == null || sv.insns.isEmpty())
            return null;
        Map<AbstractInsnNode, Integer> insnIndexMap = new IdentityHashMap<>(insns.length);
        for (int k = 0; k < insns.length; k++)
            insnIndexMap.put(insns[k], k);
        TypeInsnNode found = null;
        Deque<AbstractInsnNode> queue = new ArrayDeque<>(sv.insns);
        Set<AbstractInsnNode> visited = Collections.newSetFromMap(new IdentityHashMap<>());
        while (!queue.isEmpty() && visited.size() < 48) {
            AbstractInsnNode origin = queue.poll();
            if (!visited.add(origin))
                continue;
            int op = origin.getOpcode();
            if (op == Opcodes.NEW && origin instanceof TypeInsnNode) {
                TypeInsnNode tin = (TypeInsnNode) origin;
                if (!expectedNewInternal.equals(tin.desc))
                    return null;
                if (found != null && found != tin)
                    return null;
                found = tin;
            } else if (op == Opcodes.DUP || op == Opcodes.DUP_X1 || op == Opcodes.DUP_X2 || op == Opcodes.CHECKCAST) {
                Integer idx = insnIndexMap.get(origin);
                if (idx == null || idx >= srcFrames.length || srcFrames[idx] == null)
                    return null;
                int sp = srcFrames[idx].getStackSize();
                if (sp <= 0)
                    return null;
                SourceValue top = srcFrames[idx].getStack(sp - 1);
                if (top == null || top.insns == null || top.insns.isEmpty())
                    return null;
                queue.addAll(top.insns);
            } else if (origin instanceof VarInsnNode && op == Opcodes.ALOAD) {
                int varIdx = ((VarInsnNode) origin).var;
                Integer aloadIdx = insnIndexMap.get(origin);
                if (aloadIdx == null)
                    return null;
                List<Integer> stores = findReachingStores(insns, aloadIdx, varIdx);
                if (stores.isEmpty())
                    return null;
                for (int k : stores) {
                    if (srcFrames[k] == null)
                        return null;
                    int ksp = srcFrames[k].getStackSize();
                    if (ksp <= 0)
                        return null;
                    SourceValue stored = srcFrames[k].getStack(ksp - 1);
                    if (stored == null || stored.insns == null || stored.insns.isEmpty())
                        return null;
                    queue.addAll(stored.insns);
                }
            } else {
                return null;
            }
        }
        return found;
    }

    private String findUniqueConcreteSubclass(String superName, String ifaceInternal, String ctorDesc) {
        List<String> candidates = new ArrayList<>();
        Map<String, Integer> candidateDepths = new HashMap<>();
        for (String clsDot : rawClasses.keySet()) {
            String cand = clsDot.replace('.', '/');
            if (cand.equals(superName) || !isConcreteRawClass(cand))
                continue;
            if (!isSubclassOf(cand, superName))
                continue;
            if (!isSubclassOf(cand, ifaceInternal))
                continue;
            List<String> chain = new ArrayList<>();
            if (!canSatisfySubClassInitChain(cand, superName, ctorDesc, chain))
                continue;
            boolean declaredDifferent = false;
            for (String stepCls : chain) {
                Set<String> stepInits = classDeclaredInits.get(stepCls);
                if (stepInits != null && !stepInits.isEmpty() && !stepInits.contains(ctorDesc)) {
                    declaredDifferent = true;
                    break;
                }
            }
            if (declaredDifferent)
                continue;
            candidates.add(cand);
            candidateDepths.put(cand, chain.size());
        }
        if (candidates.size() == 1) {
            return candidates.get(0);
        }
        if (candidates.size() > 1) {
            int minDepth = Integer.MAX_VALUE;
            for (String cand : candidates) {
                int d = candidateDepths.getOrDefault(cand, Integer.MAX_VALUE);
                if (d < minDepth)
                    minDepth = d;
            }
            List<String> shallowest = new ArrayList<>();
            for (String cand : candidates) {
                if (candidateDepths.getOrDefault(cand, Integer.MAX_VALUE) == minDepth) {
                    shallowest.add(cand);
                }
            }
            if (shallowest.size() == 1) {
                return shallowest.get(0);
            }
            List<String> noOverride = new ArrayList<>();
            for (String cand : shallowest) {
                if (!classDeclaredInits.getOrDefault(cand, Collections.emptySet()).contains(ctorDesc)) {
                    noOverride.add(cand);
                }
            }
            if (noOverride.size() == 1) {
                return noOverride.get(0);
            }
        }
        return null;
    }

    private String findUniqueConcreteImplWithZeroArgInit(String ifaceInternal) {
        if (ifaceInternal == null || !rawClasses.containsKey(ifaceInternal.replace('/', '.')))
            return null;
        List<String> candidates = new ArrayList<>();
        for (String clsDot : rawClasses.keySet()) {
            String cand = clsDot.replace('.', '/');
            if (!isConcreteRawClass(cand))
                continue;
            if (!isSubclassOf(cand, ifaceInternal))
                continue;
            if (!canProvideZeroArgInit(cand))
                continue;
            candidates.add(cand);
        }
        return candidates.size() == 1 ? candidates.get(0) : null;
    }

    private boolean tryUnmergeSuperNew(
            MethodNode mn,
            Frame<SourceValue>[] srcFrames,
            AbstractInsnNode[] insns,
            int insnIdx,
            int valueDepthFromTop,
            BasicValue bv,
            String wantSub) {
        if (srcFrames == null || bv == null || bv.getType() == null || wantSub == null)
            return false;
        org.objectweb.asm.Type t = bv.getType();
        if (t.getSort() != org.objectweb.asm.Type.OBJECT)
            return false;
        String superName = t.getInternalName();
        if (superName.equals(wantSub))
            return false;

        boolean wantIsInterface = isRawInterface(wantSub);
        if (wantIsInterface) {
            if ("java/lang/Object".equals(superName) || isSubclassOf(superName, wantSub))
                return false;
        } else {
            if (!isSubclassOf(wantSub, superName))
                return false;
            if (!rawClasses.containsKey(wantSub.replace('/', '.')))
                return false;
        }

        if (insnIdx >= srcFrames.length || srcFrames[insnIdx] == null)
            return false;
        int sp = srcFrames[insnIdx].getStackSize();
        if (valueDepthFromTop < 0 || valueDepthFromTop >= sp)
            return false;

        TypeInsnNode tin = traceSingleNewAllocation(srcFrames[insnIdx].getStack(sp - 1 - valueDepthFromTop), srcFrames,
                insns, superName);
        if (tin == null)
            return false;

        MethodInsnNode matchingInit = null;
        for (int k = 0; k < insns.length && k < srcFrames.length; k++) {
            if (insns[k] instanceof MethodInsnNode && insns[k].getOpcode() == Opcodes.INVOKESPECIAL) {
                MethodInsnNode min = (MethodInsnNode) insns[k];
                if ("<init>".equals(min.name) && superName.equals(min.owner) && srcFrames[k] != null) {
                    int argc = org.objectweb.asm.Type.getArgumentTypes(min.desc).length;
                    int ksp = srcFrames[k].getStackSize();
                    if (ksp > argc) {
                        SourceValue recv = srcFrames[k].getStack(ksp - 1 - argc);
                        if (traceSingleNewAllocation(recv, srcFrames, insns, superName) == tin) {
                            matchingInit = min;
                            break;
                        }
                    }
                }
            }
        }
        if (matchingInit == null)
            return false;

        String targetSub = wantSub;
        if (wantIsInterface) {
            targetSub = findUniqueConcreteSubclass(superName, wantSub, matchingInit.desc);
            if (targetSub == null)
                return false;
        }

        if (!ensureSubClassInitChain(targetSub, superName, matchingInit.desc))
            return false;

        tin.desc = targetSub;
        matchingInit.owner = targetSub;
        classesInstantiatedWithNew.add(targetSub);
        return true;
    }

    private void repairMethodTypesWithAnalyzer(String internalName, ClassNode cn, MethodNode mn) {
        try {
            org.objectweb.asm.Type ownerType = org.objectweb.asm.Type.getObjectType(internalName);
            org.objectweb.asm.Type superType = cn.superName != null ? org.objectweb.asm.Type.getObjectType(cn.superName)
                    : null;
            boolean isIf = (cn.access & Opcodes.ACC_INTERFACE) != 0;

            for (int pass = 0; pass < 2; pass++) {
                Analyzer<BasicValue> analyzer = new Analyzer<>(
                        new SimpleVerifier(Opcodes.ASM9, ownerType, superType, null, isIf) {
                            @Override
                            protected boolean isInterface(org.objectweb.asm.Type type) {
                                if (type == null || type.getSort() != org.objectweb.asm.Type.OBJECT)
                                    return false;
                                if ("java/lang/Object".equals(type.getInternalName()))
                                    return false;
                                return isRawInterface(type.getInternalName());
                            }

                            @Override
                            protected org.objectweb.asm.Type getSuperClass(org.objectweb.asm.Type type) {
                                if (type == null || type.getSort() != org.objectweb.asm.Type.OBJECT) {
                                    return org.objectweb.asm.Type.getObjectType("java/lang/Object");
                                }
                                String s = classSuperMap.get(type.getInternalName());
                                if (s != null)
                                    return org.objectweb.asm.Type.getObjectType(s);
                                try {
                                    Class<?> c = Class.forName(type.getClassName(), false,
                                            NanoClassLoader.this.getParent());
                                    return c.getSuperclass() != null ? org.objectweb.asm.Type.getType(c.getSuperclass())
                                            : null;
                                } catch (Throwable ignored) {
                                    return org.objectweb.asm.Type.getObjectType("java/lang/Object");
                                }
                            }

                            @Override
                            protected boolean isAssignableFrom(org.objectweb.asm.Type type1,
                                    org.objectweb.asm.Type type2) {
                                if (type1 == null || type2 == null)
                                    return false;
                                if (type1.equals(type2))
                                    return true;
                                if (type1.getSort() != org.objectweb.asm.Type.OBJECT
                                        && type1.getSort() != org.objectweb.asm.Type.ARRAY) {
                                    return type1.equals(type2);
                                }
                                if (type2.getSort() != org.objectweb.asm.Type.OBJECT
                                        && type2.getSort() != org.objectweb.asm.Type.ARRAY) {
                                    return false;
                                }
                                String i1 = type1.getInternalName();
                                String i2 = type2.getInternalName();
                                if ("java/lang/Object".equals(i1) || "null".equals(i2) || i1.equals(i2))
                                    return true;
                                if (isSubclassOf(i2, i1))
                                    return true;
                                if (rawClasses.containsKey(i1.replace('/', '.'))
                                        || rawClasses.containsKey(i2.replace('/', '.'))) {
                                    return false;
                                }
                                return super.isAssignableFrom(type1, type2);
                            }

                            @Override
                            protected boolean isArrayValue(BasicValue value) {
                                if (value != null && value.getType() != null) {
                                    int s = value.getType().getSort();
                                    return s == org.objectweb.asm.Type.ARRAY
                                            || "java/lang/Object".equals(value.getType().getInternalName());
                                }
                                return true;
                            }

                            @Override
                            protected BasicValue getElementValue(BasicValue objectArrayValue) throws AnalyzerException {
                                if (objectArrayValue != null && objectArrayValue.getType() != null
                                        && objectArrayValue.getType().getSort() == org.objectweb.asm.Type.ARRAY) {
                                    return super.getElementValue(objectArrayValue);
                                }
                                return BasicValue.REFERENCE_VALUE;
                            }

                            @Override
                            protected boolean isSubTypeOf(BasicValue value, BasicValue expected) {
                                return true;
                            }

                            @Override
                            protected Class<?> getClass(org.objectweb.asm.Type t) {
                                try {
                                    return Class.forName(t.getClassName(), false, NanoClassLoader.this.getParent());
                                } catch (Throwable e) {
                                    return Object.class;
                                }
                            }
                        });
                Frame<BasicValue>[] frames = analyzer.analyze(internalName, mn);
                AbstractInsnNode[] insns = mn.instructions.toArray();
                Frame<SourceValue>[] srcFrames = null;

                if (pass == 0) {
                    boolean unmergedAny = false;
                    for (int i = 0; i < insns.length && i < frames.length; i++) {
                        Frame<BasicValue> f = frames[i];
                        if (f == null)
                            continue;
                        int sp = f.getStackSize();
                        if (sp <= 0)
                            continue;
                        AbstractInsnNode ins = insns[i];
                        if (ins instanceof FieldInsnNode && (ins.getOpcode() == Opcodes.PUTFIELD
                                || ins.getOpcode() == Opcodes.PUTSTATIC || ins.getOpcode() == Opcodes.GETFIELD)) {
                            FieldInsnNode fin = (FieldInsnNode) ins;
                            if (ins.getOpcode() == Opcodes.GETFIELD) {
                                BasicValue top = f.getStack(sp - 1);
                                if (shouldCastTo(top, fin.owner, internalName, mn.name)
                                        && (isSubclassOf(fin.owner, top.getType().getInternalName())
                                                || isRawInterface(fin.owner))) {
                                    if (srcFrames == null)
                                        srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName, mn);
                                    if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 0, top, fin.owner))
                                        unmergedAny = true;
                                }
                            } else if (ins.getOpcode() == Opcodes.PUTFIELD && sp >= 2) {
                                BasicValue obj = f.getStack(sp - 2);
                                if (shouldCastTo(obj, fin.owner, internalName, mn.name)
                                        && (isSubclassOf(fin.owner, obj.getType().getInternalName())
                                                || isRawInterface(fin.owner))) {
                                    if (srcFrames == null)
                                        srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName, mn);
                                    if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 1, obj, fin.owner))
                                        unmergedAny = true;
                                }
                                if (fin.desc.startsWith("L") && fin.desc.endsWith(";")) {
                                    String want = fin.desc.substring(1, fin.desc.length() - 1);
                                    BasicValue val = f.getStack(sp - 1);
                                    if (shouldCastTo(val, want, internalName, mn.name)
                                            && (isSubclassOf(want, val.getType().getInternalName())
                                                    || isRawInterface(want))) {
                                        if (srcFrames == null)
                                            srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName,
                                                    mn);
                                        if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 0, val, want))
                                            unmergedAny = true;
                                    }
                                }
                            } else if (ins.getOpcode() == Opcodes.PUTSTATIC && fin.desc.startsWith("L")
                                    && fin.desc.endsWith(";")) {
                                String want = fin.desc.substring(1, fin.desc.length() - 1);
                                BasicValue val = f.getStack(sp - 1);
                                if (shouldCastTo(val, want, internalName, mn.name)
                                        && (isSubclassOf(want, val.getType().getInternalName())
                                                || isRawInterface(want))) {
                                    if (srcFrames == null)
                                        srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName, mn);
                                    if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 0, val, want))
                                        unmergedAny = true;
                                }
                            }
                        } else if (ins instanceof MethodInsnNode) {
                            MethodInsnNode min = (MethodInsnNode) ins;
                            org.objectweb.asm.Type[] args = org.objectweb.asm.Type.getArgumentTypes(min.desc);
                            boolean hasRecv = ins.getOpcode() != Opcodes.INVOKESTATIC;
                            int neededCount = args.length + (hasRecv ? 1 : 0);
                            if (sp >= neededCount) {
                                int valDepth = 0;
                                for (int a = args.length - 1; a >= 0; a--) {
                                    if (args[a].getSort() == org.objectweb.asm.Type.OBJECT) {
                                        BasicValue bv = f.getStack(sp - 1 - valDepth);
                                        String want = args[a].getInternalName();
                                        if (shouldCastTo(bv, want, internalName, mn.name)
                                                && (isSubclassOf(want, bv.getType().getInternalName())
                                                        || isRawInterface(want))) {
                                            if (srcFrames == null)
                                                srcFrames = new Analyzer<>(new SourceInterpreter())
                                                        .analyze(internalName, mn);
                                            if (tryUnmergeSuperNew(mn, srcFrames, insns, i, valDepth, bv, want))
                                                unmergedAny = true;
                                        }
                                    }
                                    valDepth++;
                                }
                                if (hasRecv && !"<init>".equals(min.name)) {
                                    BasicValue recv = f.getStack(sp - 1 - valDepth);
                                    if (shouldCastTo(recv, min.owner, internalName, mn.name)
                                            && (isSubclassOf(min.owner, recv.getType().getInternalName())
                                                    || isRawInterface(min.owner))) {
                                        if (srcFrames == null)
                                            srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName,
                                                    mn);
                                        if (tryUnmergeSuperNew(mn, srcFrames, insns, i, valDepth, recv, min.owner))
                                            unmergedAny = true;
                                    }
                                }
                            }
                        } else if (ins instanceof TypeInsnNode && ins.getOpcode() == Opcodes.CHECKCAST) {
                            String want = ((TypeInsnNode) ins).desc;
                            BasicValue top = f.getStack(sp - 1);
                            if (want != null && !want.startsWith("[") && shouldCastTo(top, want, internalName, mn.name)
                                    && (isSubclassOf(want, top.getType().getInternalName()) || isRawInterface(want))) {
                                if (srcFrames == null)
                                    srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName, mn);
                                if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 0, top, want))
                                    unmergedAny = true;
                            }
                        } else if (ins.getOpcode() == Opcodes.ARETURN) {
                            String ret = org.objectweb.asm.Type.getReturnType(mn.desc).getDescriptor();
                            if (ret.startsWith("L") && ret.endsWith(";")) {
                                String want = ret.substring(1, ret.length() - 1);
                                BasicValue top = f.getStack(sp - 1);
                                if (shouldCastTo(top, want, internalName, mn.name)
                                        && (isSubclassOf(want, top.getType().getInternalName())
                                                || isRawInterface(want))) {
                                    if (srcFrames == null)
                                        srcFrames = new Analyzer<>(new SourceInterpreter()).analyze(internalName, mn);
                                    if (tryUnmergeSuperNew(mn, srcFrames, insns, i, 0, top, want))
                                        unmergedAny = true;
                                }
                            }
                        }
                    }
                    if (unmergedAny)
                        continue;
                }

                for (int i = 0; i < insns.length && i < frames.length; i++) {
                    Frame<BasicValue> f = frames[i];
                    if (f == null)
                        continue;
                    AbstractInsnNode ins = insns[i];
                    int sp = f.getStackSize();
                    if (sp <= 0)
                        continue;

                    if (ins instanceof FieldInsnNode && (ins.getOpcode() == Opcodes.PUTSTATIC
                            || ins.getOpcode() == Opcodes.PUTFIELD || ins.getOpcode() == Opcodes.GETFIELD)) {
                        FieldInsnNode fin = (FieldInsnNode) ins;
                        if (ins.getOpcode() == Opcodes.GETFIELD) {
                            BasicValue top = f.getStack(sp - 1);
                            if (shouldCastTo(top, fin.owner, internalName, mn.name)) {
                                insertCheckcastAtStackOffset(mn, ins, 0, fin.owner, f);
                            }
                        } else if (ins.getOpcode() == Opcodes.PUTFIELD && sp >= 2) {
                            BasicValue obj = f.getStack(sp - 2);
                            if (shouldCastTo(obj, fin.owner, internalName, mn.name)) {
                                insertCheckcastAtStackOffset(mn, ins, 1, fin.owner, f);
                            }
                            if (fin.desc.startsWith("L") && fin.desc.endsWith(";")) {
                                String want = fin.desc.substring(1, fin.desc.length() - 1);
                                BasicValue val = f.getStack(sp - 1);
                                if (shouldCastTo(val, want, internalName, mn.name)) {
                                    insertCheckcastAtStackOffset(mn, ins, 0, want, f);
                                }
                            }
                        } else if (ins.getOpcode() == Opcodes.PUTSTATIC && fin.desc.startsWith("L")
                                && fin.desc.endsWith(";")) {
                            String want = fin.desc.substring(1, fin.desc.length() - 1);
                            BasicValue val = f.getStack(sp - 1);
                            if (shouldCastTo(val, want, internalName, mn.name)) {
                                insertCheckcastAtStackOffset(mn, ins, 0, want, f);
                            }
                        }
                    } else if (ins instanceof MethodInsnNode) {
                        MethodInsnNode min = (MethodInsnNode) ins;
                        org.objectweb.asm.Type[] args = org.objectweb.asm.Type.getArgumentTypes(min.desc);
                        boolean hasRecv = ins.getOpcode() != Opcodes.INVOKESTATIC;
                        int neededCount = args.length + (hasRecv ? 1 : 0);
                        if (sp >= neededCount) {
                            int valDepth = 0;
                            for (int a = args.length - 1; a >= 0; a--) {
                                if (args[a].getSort() == org.objectweb.asm.Type.OBJECT
                                        || args[a].getSort() == org.objectweb.asm.Type.ARRAY) {
                                    BasicValue bv = f.getStack(sp - 1 - valDepth);
                                    String want = args[a].getInternalName();
                                    if (shouldCastTo(bv, want, internalName, mn.name)) {
                                        insertCheckcastAtStackOffset(mn, ins, valDepth, want, f);
                                    }
                                }
                                valDepth++;
                            }
                            if (hasRecv && !"<init>".equals(min.name)) {
                                BasicValue recv = f.getStack(sp - 1 - valDepth);
                                if (shouldCastTo(recv, min.owner, internalName, mn.name)) {
                                    insertCheckcastAtStackOffset(mn, ins, valDepth, min.owner, f);
                                }
                            }
                        }
                    } else if (ins.getOpcode() == Opcodes.ARETURN) {
                        String ret = org.objectweb.asm.Type.getReturnType(mn.desc).getDescriptor();
                        if (ret.startsWith("L") && ret.endsWith(";")) {
                            String want = ret.substring(1, ret.length() - 1);
                            BasicValue top = f.getStack(sp - 1);
                            if (shouldCastTo(top, want, internalName, mn.name)) {
                                insertCheckcastAtStackOffset(mn, ins, 0, want, f);
                            }
                        }
                    }
                }
                break;
            }
        } catch (LinkageError le) {
            throw le;
        } catch (Throwable ignored) {
        }
    }

    private byte[] transformClass(String name, byte[] classBytes) {
        try {
            ClassReader cr = new ClassReader(classBytes);
            ClassNode cn = new ClassNode();
            cr.accept(cn, ClassReader.SKIP_FRAMES);

            String internalName = name.replace('.', '/');
            boolean isInterface = (cn.access & Opcodes.ACC_INTERFACE) != 0;
            boolean strippedAbstract = false;
            if (!isInterface && (cn.access & Opcodes.ACC_ABSTRACT) != 0
                    && classesInstantiatedWithNew.contains(internalName)) {
                cn.access &= ~Opcodes.ACC_ABSTRACT;
                strippedAbstract = true;
            }

            Set<String> declaredInits = new HashSet<>();
            boolean needsJava8Frames = isInterface;

            for (MethodNode mn : cn.methods) {
                if ("<init>".equals(mn.name)) {
                    declaredInits.add(mn.desc);
                }
                if (strippedAbstract && (mn.access & Opcodes.ACC_ABSTRACT) != 0) {
                    mn.access &= ~Opcodes.ACC_ABSTRACT;
                    mn.instructions.clear();
                    mn.tryCatchBlocks.clear();
                    mn.localVariables = null;
                    org.objectweb.asm.Type retType = org.objectweb.asm.Type.getReturnType(mn.desc);
                    if (retType.getSort() == org.objectweb.asm.Type.VOID) {
                        mn.instructions.add(new InsnNode(Opcodes.RETURN));
                    } else if (retType.getSort() == org.objectweb.asm.Type.OBJECT
                            && "java/util/Random".equals(retType.getInternalName())) {
                        mn.instructions
                                .add(new MethodInsnNode(Opcodes.INVOKESTATIC, "java/util/concurrent/ThreadLocalRandom",
                                        "current", "()Ljava/util/concurrent/ThreadLocalRandom;", false));
                        mn.instructions.add(new InsnNode(Opcodes.ARETURN));
                    } else if (retType.getSort() == org.objectweb.asm.Type.OBJECT
                            || retType.getSort() == org.objectweb.asm.Type.ARRAY) {
                        mn.instructions.add(new InsnNode(Opcodes.ACONST_NULL));
                        mn.instructions.add(new InsnNode(Opcodes.ARETURN));
                    } else if (retType.getSort() == org.objectweb.asm.Type.LONG) {
                        mn.instructions.add(new InsnNode(Opcodes.LCONST_0));
                        mn.instructions.add(new InsnNode(Opcodes.LRETURN));
                    } else if (retType.getSort() == org.objectweb.asm.Type.FLOAT) {
                        mn.instructions.add(new InsnNode(Opcodes.FCONST_0));
                        mn.instructions.add(new InsnNode(Opcodes.FRETURN));
                    } else if (retType.getSort() == org.objectweb.asm.Type.DOUBLE) {
                        mn.instructions.add(new InsnNode(Opcodes.DCONST_0));
                        mn.instructions.add(new InsnNode(Opcodes.DRETURN));
                    } else {
                        mn.instructions.add(new InsnNode(Opcodes.ICONST_0));
                        mn.instructions.add(new InsnNode(Opcodes.IRETURN));
                    }
                    mn.maxStack = 2;
                    mn.maxLocals = 8;
                    continue;
                }
                if (isInterface && !"<clinit>".equals(mn.name) && (mn.access & Opcodes.ACC_ABSTRACT) == 0) {
                    needsJava8Frames = true;
                }

                Map<Integer, TypeInsnNode> uninitNewVars = new HashMap<>();
                Set<AbstractInsnNode> skipInsns = Collections.newSetFromMap(new IdentityHashMap<>());
                boolean handledThisSuperInit = false;
                AbstractInsnNode[] insns = mn.instructions.toArray();
                for (AbstractInsnNode ins : insns) {
                    if (skipInsns.contains(ins))
                        continue;
                    if (ins instanceof InvokeDynamicInsnNode) {
                        needsJava8Frames = true;
                    } else if (ins instanceof LdcInsnNode) {
                        Object cst = ((LdcInsnNode) ins).cst;
                        if (cst instanceof Handle || cst instanceof ConstantDynamic) {
                            needsJava8Frames = true;
                        }
                    } else if (ins.getOpcode() == Opcodes.NEW && ins instanceof TypeInsnNode) {
                        TypeInsnNode tin = (TypeInsnNode) ins;
                        AbstractInsnNode next1 = nextReal(tin);
                        if (next1 instanceof VarInsnNode && next1.getOpcode() == Opcodes.ASTORE) {
                            uninitNewVars.put(((VarInsnNode) next1).var, tin);
                        } else if (next1 != null && next1.getOpcode() == Opcodes.DUP) {
                            AbstractInsnNode next2 = nextReal(next1);
                            if (next2 instanceof VarInsnNode && next2.getOpcode() == Opcodes.ASTORE) {
                                uninitNewVars.put(((VarInsnNode) next2).var, tin);
                            }
                        }

                        if ("java/lang/Enum".equals(tin.desc) && "java/lang/Enum".equals(cn.superName)) {
                            tin.desc = internalName;
                        } else if ("java/lang/Object".equals(tin.desc)) {
                            AbstractInsnNode dup = nextReal(tin);
                            AbstractInsnNode init = nextReal(dup);
                            AbstractInsnNode after = nextReal(init);
                            if (dup != null && dup.getOpcode() == Opcodes.DUP
                                    && hasNoLabelsOrInsnsBetween(tin, dup)
                                    && hasNoLabelsOrInsnsBetween(dup, init)
                                    && init instanceof MethodInsnNode
                                    && init.getOpcode() == Opcodes.INVOKESPECIAL
                                    && "java/lang/Object".equals(((MethodInsnNode) init).owner)
                                    && "<init>".equals(((MethodInsnNode) init).name)
                                    && "()V".equals(((MethodInsnNode) init).desc)
                                    && after != null) {
                                String target = null;
                                if (after instanceof VarInsnNode && after.getOpcode() == Opcodes.ASTORE) {
                                    target = findTargetOwnerForNewObjectVar(mn, after, ((VarInsnNode) after).var);
                                } else {
                                    target = findTargetOwnerForStackTop(mn, after);
                                }
                                boolean useInterfaceProxy = false;
                                if (target != null && !target.startsWith("[") && isRawInterface(target)) {
                                    if (isSubclassOf(internalName, target) && canProvideZeroArgInit(internalName)) {
                                        target = internalName;
                                    } else {
                                        String uniqueImpl = findUniqueConcreteImplWithZeroArgInit(target);
                                        if (uniqueImpl != null) {
                                            target = uniqueImpl;
                                        } else {
                                            useInterfaceProxy = true;
                                        }
                                    }
                                }
                                if (useInterfaceProxy) {
                                    mn.instructions.set(tin,
                                            new LdcInsnNode(org.objectweb.asm.Type.getObjectType(target)));
                                    mn.instructions.remove(dup);
                                    MethodInsnNode proxyCall = new MethodInsnNode(
                                            Opcodes.INVOKESTATIC,
                                            "nanoandroid/host/NanoClassLoader",
                                            "createDummyInterfaceProxy",
                                            "(Ljava/lang/Class;)Ljava/lang/Object;",
                                            false);
                                    mn.instructions.set(init, proxyCall);
                                    mn.instructions.insert(proxyCall, new TypeInsnNode(Opcodes.CHECKCAST, target));
                                    skipInsns.add(dup);
                                    skipInsns.add(init);
                                } else if (target != null && !"java/lang/Object".equals(target)
                                        && !isRawInterface(target)
                                        && rawClasses.containsKey(target.replace('/', '.'))
                                        && canProvideZeroArgInit(target)) {
                                    tin.desc = target;
                                    ((MethodInsnNode) init).owner = target;
                                    classesInstantiatedWithNew.add(target);
                                    classNeededInits.computeIfAbsent(target, k -> new HashSet<>()).add("()V");
                                }
                            }
                        } else if (!"java/lang/Object".equals(tin.desc)
                                && !isRawInterface(tin.desc)
                                && rawClasses.containsKey(tin.desc.replace('/', '.'))
                                && canProvideZeroArgInit(tin.desc)) {
                            AbstractInsnNode dup = nextReal(tin);
                            AbstractInsnNode init = nextReal(dup);
                            if (dup != null && dup.getOpcode() == Opcodes.DUP
                                    && init instanceof MethodInsnNode
                                    && init.getOpcode() == Opcodes.INVOKESPECIAL
                                    && "java/lang/Object".equals(((MethodInsnNode) init).owner)
                                    && "<init>".equals(((MethodInsnNode) init).name)
                                    && "()V".equals(((MethodInsnNode) init).desc)) {
                                ((MethodInsnNode) init).owner = tin.desc;
                                classNeededInits.computeIfAbsent(tin.desc, k -> new HashSet<>()).add("()V");
                            }
                        }
                    } else if (ins instanceof MethodInsnNode) {
                        MethodInsnNode min = (MethodInsnNode) ins;
                        if (min.getOpcode() == Opcodes.INVOKESPECIAL && "<init>".equals(min.name)) {
                            if ("java/lang/Enum".equals(min.owner) && "(Ljava/lang/String;I)V".equals(min.desc)
                                    && "java/lang/Enum".equals(cn.superName) && !"<init>".equals(mn.name)) {
                                min.owner = internalName;
                            } else if ("<init>".equals(mn.name) && !handledThisSuperInit && "()V".equals(min.desc)) {
                                AbstractInsnNode prev = min.getPrevious();
                                while (prev != null && (prev instanceof LabelNode || prev instanceof LineNumberNode
                                        || prev instanceof FrameNode)) {
                                    prev = prev.getPrevious();
                                }
                                if (prev instanceof VarInsnNode && prev.getOpcode() == Opcodes.ALOAD
                                        && ((VarInsnNode) prev).var == 0) {
                                    handledThisSuperInit = true;
                                    if (cn.superName != null && !min.owner.equals(internalName)
                                            && !min.owner.equals(cn.superName)
                                            && isSubclassOf(cn.superName, min.owner)
                                            && canProvideZeroArgInit(cn.superName)) {
                                        min.owner = cn.superName;
                                        classNeededInits.computeIfAbsent(cn.superName, k -> new HashSet<>()).add("()V");
                                    }
                                }
                            } else {
                                org.objectweb.asm.Type[] args = org.objectweb.asm.Type.getArgumentTypes(min.desc);
                                AbstractInsnNode back = min.getPrevious();
                                int needed = args.length;
                                for (int steps = 0; back != null && needed > 0 && steps < 64; steps++) {
                                    if (back instanceof LabelNode || back instanceof LineNumberNode
                                            || back instanceof FrameNode) {
                                        back = back.getPrevious();
                                        continue;
                                    }
                                    if (back instanceof JumpInsnNode || back instanceof TableSwitchInsnNode
                                            || back instanceof LookupSwitchInsnNode) {
                                        back = null;
                                        break;
                                    }
                                    int[] eff = stackEffectValues(back);
                                    if (eff == null) {
                                        back = null;
                                        break;
                                    }
                                    needed -= (eff[1] - eff[0]);
                                    if (needed < 0) {
                                        back = null;
                                        break;
                                    }
                                    back = back.getPrevious();
                                }
                                if (needed != 0)
                                    back = null;
                                while (back != null && (back instanceof LabelNode || back instanceof LineNumberNode
                                        || back instanceof FrameNode)) {
                                    back = back.getPrevious();
                                }
                                TypeInsnNode newTin = null;
                                if ("<init>".equals(mn.name) && !handledThisSuperInit
                                        && back instanceof VarInsnNode && back.getOpcode() == Opcodes.ALOAD
                                        && ((VarInsnNode) back).var == 0) {
                                    handledThisSuperInit = true;
                                    if (cn.superName != null && !min.owner.equals(internalName)
                                            && !min.owner.equals(cn.superName)
                                            && isSubclassOf(cn.superName, min.owner)
                                            && ensureSubClassInitChain(cn.superName, min.owner, min.desc)) {
                                        min.owner = cn.superName;
                                    }
                                } else if (back instanceof VarInsnNode && back.getOpcode() == Opcodes.ALOAD) {
                                    newTin = uninitNewVars.get(((VarInsnNode) back).var);
                                } else if (back != null && back.getOpcode() == Opcodes.DUP) {
                                    AbstractInsnNode prevNew = back.getPrevious();
                                    while (prevNew != null && (prevNew instanceof LabelNode
                                            || prevNew instanceof LineNumberNode || prevNew instanceof FrameNode)) {
                                        prevNew = prevNew.getPrevious();
                                    }
                                    if (prevNew != null && prevNew.getOpcode() == Opcodes.NEW
                                            && prevNew instanceof TypeInsnNode) {
                                        newTin = (TypeInsnNode) prevNew;
                                    }
                                }
                                if (newTin != null && !min.owner.equals(newTin.desc)
                                        && isSubclassOf(newTin.desc, min.owner)
                                        && rawClasses.containsKey(newTin.desc.replace('/', '.'))
                                        && ensureSubClassInitChain(newTin.desc, min.owner, min.desc)) {
                                    String targetSub = newTin.desc;
                                    min.owner = targetSub;
                                    classNeededInits.computeIfAbsent(targetSub, k -> new HashSet<>()).add(min.desc);
                                }
                            }
                        } else if (min.getOpcode() == Opcodes.INVOKEINTERFACE && "android/view/Menu".equals(min.owner)
                                && "findItem".equals(min.name) && "(I)Landroid/view/MenuItem;".equals(min.desc)) {
                            min.setOpcode(Opcodes.INVOKESTATIC);
                            min.owner = "nanoandroid/host/NanoClassLoader";
                            min.name = "findOrCreateMenuItem";
                            min.desc = "(Landroid/view/Menu;I)Landroid/view/MenuItem;";
                            min.itf = false;
                        }
                    }
                }

                repairMethodTypesWithAnalyzer(internalName, cn, mn);
            }

            if (!isInterface && cn.superName != null) {
                boolean hadNoDeclaredInits = declaredInits.isEmpty();
                if ("java/lang/Enum".equals(cn.superName)) {
                    if (!declaredInits.contains("(Ljava/lang/String;I)V")) {
                        injectForwardingInit(cn, "java/lang/Enum", "(Ljava/lang/String;I)V");
                        declaredInits.add("(Ljava/lang/String;I)V");
                    }
                } else {
                    if (hadNoDeclaredInits) {
                        Set<String> superDeclared = null;
                        String walkSuper = cn.superName;
                        for (int hop = 0; hop < 12 && walkSuper != null
                                && !"java/lang/Object".equals(walkSuper); hop++) {
                            Set<String> candInits = classDeclaredInits.get(walkSuper);
                            if (candInits != null && !candInits.isEmpty()) {
                                superDeclared = candInits;
                                break;
                            }
                            if (!rawClasses.containsKey(walkSuper.replace('/', '.')))
                                break;
                            walkSuper = classSuperMap.get(walkSuper);
                        }
                        if (superDeclared != null) {
                            for (String desc : new ArrayList<>(superDeclared)) {
                                if (!declaredInits.contains(desc) && !"()V".equals(desc)) {
                                    injectForwardingInit(cn, cn.superName, desc);
                                    declaredInits.add(desc);
                                }
                            }
                        }
                    }
                    if (!declaredInits.contains("()V")
                            && (hadNoDeclaredInits || classNeededInits
                                    .getOrDefault(internalName, Collections.emptySet()).contains("()V"))
                            && hasSuperInit(cn.superName, "()V")) {
                        injectForwardingInit(cn, cn.superName, "()V");
                        declaredInits.add("()V");
                    }
                }

                Set<String> needed = classNeededInits.get(internalName);
                if (needed != null) {
                    for (String desc : needed) {
                        if (!declaredInits.contains(desc)
                                && !("java/lang/Enum".equals(cn.superName) && "()V".equals(desc))
                                && hasSuperInit(cn.superName, desc)) {
                            injectForwardingInit(cn, cn.superName, desc);
                            declaredInits.add(desc);
                        }
                    }
                }
            }

            if (!needsJava8Frames) {
                // Downgrading class version to V1_5 triggers JVM classic type-inference
                // verifier:
                // still fully enforces memory & field type safety, without spurious
                // missing-StackMapTable VerifyErrors from dex2jar.
                cn.version = Opcodes.V1_5;
                ClassWriter cw = new ClassWriter(ClassWriter.COMPUTE_MAXS);
                cn.accept(cw);
                return cw.toByteArray();
            } else {
                cn.version = Math.max(cn.version, Opcodes.V1_8);
                ClassWriter cw = new ClassWriter(ClassWriter.COMPUTE_FRAMES) {
                    @Override
                    protected String getCommonSuperClass(String type1, String type2) {
                        return resolveCommonSuper(type1, type2);
                    }
                };
                cn.accept(cw);
                return cw.toByteArray();
            }
        } catch (Throwable t) {
            return classBytes;
        }
    }

    private void injectForwardingInit(ClassNode cn, String superInternalName, String descriptor) {
        org.objectweb.asm.Type[] args = org.objectweb.asm.Type.getArgumentTypes(descriptor);
        MethodNode mv = new MethodNode(Opcodes.ACC_PUBLIC, "<init>", descriptor, null, null);
        mv.visitCode();
        mv.visitVarInsn(Opcodes.ALOAD, 0);

        int localIdx = 1;
        for (org.objectweb.asm.Type arg : args) {
            switch (arg.getSort()) {
                case org.objectweb.asm.Type.BOOLEAN:
                case org.objectweb.asm.Type.BYTE:
                case org.objectweb.asm.Type.CHAR:
                case org.objectweb.asm.Type.SHORT:
                case org.objectweb.asm.Type.INT:
                    mv.visitVarInsn(Opcodes.ILOAD, localIdx);
                    localIdx += 1;
                    break;
                case org.objectweb.asm.Type.LONG:
                    mv.visitVarInsn(Opcodes.LLOAD, localIdx);
                    localIdx += 2;
                    break;
                case org.objectweb.asm.Type.FLOAT:
                    mv.visitVarInsn(Opcodes.FLOAD, localIdx);
                    localIdx += 1;
                    break;
                case org.objectweb.asm.Type.DOUBLE:
                    mv.visitVarInsn(Opcodes.DLOAD, localIdx);
                    localIdx += 2;
                    break;
                default:
                    mv.visitVarInsn(Opcodes.ALOAD, localIdx);
                    localIdx += 1;
                    break;
            }
        }

        mv.visitMethodInsn(Opcodes.INVOKESPECIAL, superInternalName, "<init>", descriptor, false);
        mv.visitInsn(Opcodes.RETURN);
        mv.visitMaxs(localIdx + 1, localIdx + 1);
        mv.visitEnd();
        cn.methods.add(mv);
    }
}
