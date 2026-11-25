package android.text.format;

import android.content.Context;
import java.util.Locale;
import java.util.Date;
import java.util.Calendar;
import java.text.SimpleDateFormat;

public class DateFormat {
    public DateFormat() {}

    public static boolean is24HourFormat(Context context) {
        return true;
    }

    public static String getBestDateTimePattern(Locale locale, String skeleton) {
        return skeleton;
    }

    public static java.text.DateFormat getTimeFormat(Context context) {
        return new SimpleDateFormat("HH:mm", Locale.getDefault());
    }

    public static java.text.DateFormat getDateFormat(Context context) {
        return new SimpleDateFormat("yyyy-MM-dd", Locale.getDefault());
    }

    public static java.text.DateFormat getLongDateFormat(Context context) {
        return new SimpleDateFormat("MMMM d, yyyy", Locale.getDefault());
    }

    public static java.text.DateFormat getMediumDateFormat(Context context) {
        return new SimpleDateFormat("MMM d, yyyy", Locale.getDefault());
    }

    public static char[] getDateFormatOrder(Context context) {
        return new char[]{'y', 'M', 'd'};
    }

    public static CharSequence format(CharSequence inFormat, long inTimeInMillis) {
        return format(inFormat, new Date(inTimeInMillis));
    }

    public static CharSequence format(CharSequence inFormat, Date inDate) {
        if (inFormat == null || inDate == null) return "";
        try {
            return new SimpleDateFormat(inFormat.toString(), Locale.getDefault()).format(inDate);
        } catch (Throwable t) {
            return inDate.toString();
        }
    }

    public static CharSequence format(CharSequence inFormat, Calendar inCal) {
        return inCal != null ? format(inFormat, inCal.getTime()) : "";
    }
}
