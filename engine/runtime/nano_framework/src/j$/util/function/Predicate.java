package j$.util.function;

@FunctionalInterface
public interface Predicate<T> extends java.util.function.Predicate<T> {
    @Override
    boolean test(T t);
}
