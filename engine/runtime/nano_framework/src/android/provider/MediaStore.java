package android.provider;

public class MediaStore {
    public static class MediaColumns {
        public static final String DATA = "_data";
        public static final String DISPLAY_NAME = "_display_name";
        public static final String SIZE = "_size";
    }
    public static class Video {
        public static class Media extends MediaColumns {}
    }
}
