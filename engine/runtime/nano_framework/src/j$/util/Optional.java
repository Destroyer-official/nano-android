package j$.util;

import java.util.NoSuchElementException;
import java.util.Objects;

public final class Optional<T> {
    private static final Optional<?> EMPTY = new Optional<>(null);
    private final T value;

    private Optional(T value) { this.value = value; }
    @SuppressWarnings("unchecked")
    public static <T> Optional<T> empty() { return (Optional<T>) EMPTY; }
    public static <T> Optional<T> of(T value) { return new Optional<>(Objects.requireNonNull(value)); }
    public static <T> Optional<T> ofNullable(T value) { return value == null ? empty() : of(value); }

    public boolean isPresent() { return value != null; }
    public boolean isEmpty() { return value == null; }
    public T get() {
        if (value == null) throw new NoSuchElementException("No value present");
        return value;
    }
    public T orElse(T other) { return value != null ? value : other; }
    public <U> Optional<U> map(j$.util.function.Function<? super T, ? extends U> mapper) {
        Objects.requireNonNull(mapper);
        return isPresent() ? ofNullable(mapper.apply(value)) : empty();
    }
    public void ifPresent(j$.util.function.Consumer<? super T> action) {
        if (value != null) action.accept(value);
    }
}
