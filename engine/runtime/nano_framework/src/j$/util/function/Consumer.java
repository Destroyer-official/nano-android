package j$.util.function;

@FunctionalInterface
public interface Consumer<T> extends java.util.function.Consumer<T> {
    @Override
    void accept(T t);
}
