package android.graphics;

public class Typeface {
    public static final int NORMAL = 0;
    public static final int BOLD = 1;
    public static final int ITALIC = 2;
    public static final int BOLD_ITALIC = 3;

    public static final Typeface DEFAULT = new Typeface("SansSerif", NORMAL);
    public static final Typeface DEFAULT_BOLD = new Typeface("SansSerif", BOLD);
    public static final Typeface SANS_SERIF = new Typeface("SansSerif", NORMAL);
    public static final Typeface SERIF = new Typeface("Serif", NORMAL);
    public static final Typeface MONOSPACE = new Typeface("Monospaced", NORMAL);

    private final String familyName;
    private final int style;

    public Typeface(int style) {
        this("SansSerif", style);
    }

    public Typeface(String familyName, int style) {
        this.familyName = familyName != null ? familyName : "SansSerif";
        this.style = style;
    }

    public int getStyle() {
        return style;
    }

    public String getFamilyName() {
        return familyName;
    }

    public static Typeface create(Typeface family, int style) {
        String fam = family != null ? family.familyName : "SansSerif";
        return new Typeface(fam, style);
    }

    public static Typeface create(String familyName, int style) {
        String fam = "SansSerif";
        if (familyName != null) {
            String low = familyName.toLowerCase();
            if (low.contains("mono")) fam = "Monospaced";
            else if (low.contains("serif") && !low.contains("sans")) fam = "Serif";
        }
        return new Typeface(fam, style);
    }
}
