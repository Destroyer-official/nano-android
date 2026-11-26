package j$.util.function;

@FunctionalInterface
public interface Function<T, R> extends java.util.function.Function<T, R> {
    @Override
    R apply(T t);
}
