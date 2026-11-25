package android.graphics;

public class Color {
    public static final int BLACK       = 0xFF000000;
    public static final int DKGRAY      = 0xFF444444;
    public static final int GRAY        = 0xFF888888;
    public static final int LTGRAY      = 0xFFCCCCCC;
    public static final int WHITE       = 0xFFFFFFFF;
    public static final int RED         = 0xFFFF0000;
    public static final int GREEN       = 0xFF00FF00;
    public static final int BLUE        = 0xFF0000FF;
    public static final int YELLOW      = 0xFFFFFF00;
    public static final int CYAN        = 0xFF00FFFF;
    public static final int MAGENTA     = 0xFFFF00FF;
    public static final int TRANSPARENT = 0;

    public static int rgb(int red, int green, int blue) {
        return 0xff000000 | (red << 16) | (green << 8) | blue;
    }

    public static int argb(int alpha, int red, int green, int blue) {
        return (alpha << 24) | (red << 16) | (green << 8) | blue;
    }

    public static int red(int color) {
        return (color >> 16) & 0xFF;
    }

    public static int green(int color) {
        return (color >> 8) & 0xFF;
    }

    public static int blue(int color) {
        return color & 0xFF;
    }

    public static int alpha(int color) {
        return color >>> 24;
    }

    public static float luminance(int color) {
        return (0.2126f * red(color) + 0.7152f * green(color) + 0.0722f * blue(color)) / 255.0f;
    }

    public static void colorToHSV(int color, float[] hsv) {
        int r = red(color);
        int g = green(color);
        int b = blue(color);
        int max = Math.max(r, Math.max(g, b));
        int min = Math.min(r, Math.min(g, b));
        float delta = max - min;
        float h = 0;
        if (delta != 0) {
            if (max == r) {
                h = 60 * (((g - b) / delta) % 6);
            } else if (max == g) {
                h = 60 * (((b - r) / delta) + 2);
            } else {
                h = 60 * (((r - g) / delta) + 4);
            }
        }
        if (h < 0) h += 360;
        float s = max == 0 ? 0 : delta / max;
        float v = max / 255.0f;
        hsv[0] = h;
        hsv[1] = s;
        hsv[2] = v;
    }

    public static int HSVToColor(float[] hsv) {
        return HSVToColor(0xFF, hsv);
    }

    public static int HSVToColor(int alpha, float[] hsv) {
        float h = hsv[0] / 60.0f;
        float s = hsv[1];
        float v = hsv[2];
        float c = v * s;
        float x = c * (1 - Math.abs((h % 2) - 1));
        float m = v - c;
        float r = 0, g = 0, b = 0;
        if (h >= 0 && h < 1) { r = c; g = x; b = 0; }
        else if (h >= 1 && h < 2) { r = x; g = c; b = 0; }
        else if (h >= 2 && h < 3) { r = 0; g = c; b = x; }
        else if (h >= 3 && h < 4) { r = 0; g = x; b = c; }
        else if (h >= 4 && h < 5) { r = x; g = 0; b = c; }
        else if (h >= 5 && h < 6) { r = c; g = 0; b = x; }
        return argb(alpha, (int)((r + m) * 255), (int)((g + m) * 255), (int)((b + m) * 255));
    }

    public static int parseColor(String colorString) {
        if (colorString.charAt(0) == '#') {
            long color = Long.parseLong(colorString.substring(1), 16);
            if (colorString.length() == 7) {
                color |= 0x00000000ff000000;
            } else if (colorString.length() != 9) {
                throw new IllegalArgumentException("Unknown color");
            }
            return (int) color;
        }
        throw new IllegalArgumentException("Unknown color: " + colorString);
    }
}
