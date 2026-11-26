package j$.util.stream;

import java.util.Collection;
import java.util.Spliterator;
import java.util.stream.Stream;

public final class DesugarCollections {
    private DesugarCollections() {}

    public static <T> Stream<T> stream(Collection<T> c) {
        return c == null ? Stream.empty() : c.stream();
    }

    public static <T> Stream<T> stream(Spliterator<T> sp, boolean parallel) {
        return sp == null ? Stream.empty() : java.util.stream.StreamSupport.stream(sp, parallel);
    }
}
