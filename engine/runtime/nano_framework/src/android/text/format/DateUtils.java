package android.text.format;

import android.content.Context;
import java.util.Date;
import java.text.SimpleDateFormat;
import java.util.Locale;

public class DateUtils {
    public static final long SECOND_IN_MILLIS = 1000;
    public static final long MINUTE_IN_MILLIS = 60 * SECOND_IN_MILLIS;
    public static final long HOUR_IN_MILLIS = 60 * MINUTE_IN_MILLIS;
    public static final long DAY_IN_MILLIS = 24 * HOUR_IN_MILLIS;
    public static final long WEEK_IN_MILLIS = 7 * DAY_IN_MILLIS;
    public static final long YEAR_IN_MILLIS = 365 * DAY_IN_MILLIS;

    public static final int FORMAT_SHOW_TIME = 1;
    public static final int FORMAT_SHOW_WEEKDAY = 2;
    public static final int FORMAT_SHOW_YEAR = 4;
    public static final int FORMAT_NO_YEAR = 8;
    public static final int FORMAT_SHOW_DATE = 16;
    public static final int FORMAT_NO_MONTH_DAY = 32;
    public static final int FORMAT_12HOUR = 64;
    public static final int FORMAT_24HOUR = 128;
    public static final int FORMAT_CAP_AMPM = 256;
    public static final int FORMAT_NO_NOON = 512;
    public static final int FORMAT_CAP_NOON = 1024;
    public static final int FORMAT_NO_MIDNIGHT = 2048;
    public static final int FORMAT_CAP_MIDNIGHT = 4096;
    public static final int FORMAT_UTC = 8192;
    public static final int FORMAT_ABBREV_TIME = 16384;
    public static final int FORMAT_ABBREV_WEEKDAY = 32768;
    public static final int FORMAT_ABBREV_MONTH = 65536;
    public static final int FORMAT_NUMERIC_DATE = 131072;
    public static final int FORMAT_ABBREV_RELATIVE = 262144;
    public static final int FORMAT_ABBREV_ALL = 524288;
    public static final int FORMAT_CAP_NOON_MIDNIGHT = 2048 | 512;
    public static final int FORMAT_NO_NOON_MIDNIGHT = 2048 | 512;

    public DateUtils() {}

    public static CharSequence getRelativeTimeSpanString(long time) {
        return getRelativeTimeSpanString(time, System.currentTimeMillis(), MINUTE_IN_MILLIS);
    }

    public static CharSequence getRelativeTimeSpanString(long time, long now, long minResolution) {
        long diff = now - time;
        if (diff < MINUTE_IN_MILLIS) return "just now";
        if (diff < HOUR_IN_MILLIS) return (diff / MINUTE_IN_MILLIS) + " minutes ago";
        if (diff < DAY_IN_MILLIS) return (diff / HOUR_IN_MILLIS) + " hours ago";
        return (diff / DAY_IN_MILLIS) + " days ago";
    }

    public static CharSequence getRelativeTimeSpanString(long time, long now, long minResolution, int flags) {
        return getRelativeTimeSpanString(time, now, minResolution);
    }

    public static CharSequence getRelativeDateTimeString(Context c, long time, long minResolution, long transitionResolution, int flags) {
        return getRelativeTimeSpanString(time);
    }

    public static String formatDateTime(Context context, long millis, int flags) {
        return new SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault()).format(new Date(millis));
    }

    public static String formatDateRange(Context context, long startMillis, long endMillis, int flags) {
        return formatDateTime(context, startMillis, flags) + " - " + formatDateTime(context, endMillis, flags);
    }

    public static boolean isToday(long when) {
        long now = System.currentTimeMillis();
        return Math.abs(now - when) < DAY_IN_MILLIS;
    }
}
