package android.os;

import java.util.Locale;

public final class LocaleList {
    private static final LocaleList DEFAULT = new LocaleList(new Locale[]{Locale.US});
    private final Locale[] list;

    public LocaleList(Locale... locales) {
        this.list = locales != null && locales.length > 0 ? locales : new Locale[]{Locale.US};
    }

    public static LocaleList getDefault() {
        return DEFAULT;
    }

    public static LocaleList getEmptyLocaleList() {
        return new LocaleList(new Locale[0]);
    }

    public static LocaleList forLanguageTags(String list) {
        return DEFAULT;
    }

    public Locale get(int index) {
        return (index >= 0 && index < list.length) ? list[index] : null;
    }

    public boolean isEmpty() {
        return list.length == 0;
    }

    public int size() {
        return list.length;
    }

    public String toLanguageTags() {
        return "en-US";
    }
}
