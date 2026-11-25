package android.graphics;

import java.awt.image.BufferedImage;

public class Bitmap {
    public enum Config {
        ALPHA_8, RGB_565, ARGB_4444, ARGB_8888, RGBA_F16, HARDWARE
    }

    private final int width;
    private final int height;
    private BufferedImage awtImage;

    public Bitmap(int width, int height) {
        this.width = Math.max(1, width);
        this.height = Math.max(1, height);
    }

    public Bitmap(BufferedImage img) {
        this.awtImage = img;
        this.width = img != null ? img.getWidth() : 1;
        this.height = img != null ? img.getHeight() : 1;
    }

    public static Bitmap createBitmap(int width, int height, Config config) {
        return new Bitmap(width, height);
    }

    public static Bitmap createBitmap(int width, int height, Object config) {
        return new Bitmap(width, height);
    }

    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public BufferedImage getAwtImage() { return awtImage; }
    public void setAwtImage(BufferedImage img) { this.awtImage = img; }
}
