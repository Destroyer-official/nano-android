package android.provider;

import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "Sandboxed MediaStore metadata & MIME contracts backed by ContentResolver"
)
public class MediaStore {
    public static final String EXTRA_OUTPUT = "output";
    public static final String ACTION_IMAGE_CAPTURE = "android.media.action.IMAGE_CAPTURE";
    public static final String ACTION_VIDEO_CAPTURE = "android.media.action.VIDEO_CAPTURE";

    public static class MediaColumns {
        public static final String _ID = "_id";
        public static final String DATA = "_data";
        public static final String DISPLAY_NAME = "_display_name";
        public static final String MIME_TYPE = "mime_type";
        public static final String SIZE = "_size";
        public static final String DATE_ADDED = "date_added";
        public static final String DURATION = "duration";
    }

    public static class Images {
        public static class Media extends MediaColumns {}
    }

    public static class Audio {
        public static class Media extends MediaColumns {}
    }

    public static class Video {
        public static class Media extends MediaColumns {}
    }

    public static class Files {
        public static class FileColumns extends MediaColumns {}
    }
}
