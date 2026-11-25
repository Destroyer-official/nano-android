package android.content;

public class ComponentName {
    private final String packageName;
    private final String className;

    public ComponentName(String pkg, String cls) {
        this.packageName = pkg != null ? pkg : "";
        this.className = cls != null ? cls : "";
    }

    public ComponentName(Context pkg, Class<?> cls) {
        this(pkg != null ? pkg.getPackageName() : "", cls != null ? cls.getName() : "");
    }

    public ComponentName(Context pkg, String cls) {
        this(pkg != null ? pkg.getPackageName() : "", cls != null ? cls : "");
    }

    public String getPackageName() { return packageName; }
    public String getClassName() { return className; }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof ComponentName)) return false;
        ComponentName that = (ComponentName) o;
        return packageName.equals(that.packageName) && className.equals(that.className);
    }

    @Override
    public int hashCode() {
        return packageName.hashCode() * 31 + className.hashCode();
    }

    @Override
    public String toString() {
        return "ComponentInfo{" + packageName + "/" + className + "}";
    }
}
