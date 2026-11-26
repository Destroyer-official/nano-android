package j$.util;

import java.util.Comparator;

public final class Objects {
    private Objects() {}
    public static boolean equals(Object a, Object b) { return java.util.Objects.equals(a, b); }
    public static int hash(Object... values) { return java.util.Objects.hash(values); }
    public static int hashCode(Object o) { return java.util.Objects.hashCode(o); }
    public static <T> T requireNonNull(T obj) { return java.util.Objects.requireNonNull(obj); }
    public static <T> T requireNonNull(T obj, String msg) { return java.util.Objects.requireNonNull(obj, msg); }
    public static boolean nonNull(Object obj) { return java.util.Objects.nonNull(obj); }
    public static boolean isNull(Object obj) { return java.util.Objects.isNull(obj); }
    public static <T> int compare(T a, T b, Comparator<? super T> c) { return java.util.Objects.compare(a, b, c); }
}
