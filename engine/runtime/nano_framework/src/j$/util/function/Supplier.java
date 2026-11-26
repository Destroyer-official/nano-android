package j$.util.function;

@FunctionalInterface
public interface Supplier<T> extends java.util.function.Supplier<T> {
    @Override
    T get();
}
