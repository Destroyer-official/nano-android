package engine.render;

import java.awt.*;
import java.awt.geom.RoundRectangle2D;
import java.awt.image.BufferedImage;
import java.io.ByteArrayOutputStream;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import javax.imageio.ImageIO;

/**
 * Headless LayoutRenderer for Nano-Android.
 * Emulates AOSP LayoutLib (Bridge.java, RenderSessionImpl) and Java2D headless rasterization.
 * 
 * Runs without requiring X11, Wayland, or physical GPU hardware.
 * Executes exact measure(widthSpec, heightSpec) and layout(l, t, r, b) cycles on View hierarchies.
 */
public class LayoutRenderer {

    static {
        System.setProperty("java.awt.headless", "true");
    }

    public static final int MEASURE_SPEC_MODE_SHIFT = 30;
    public static final int MEASURE_SPEC_MODE_MASK = 0x3 << MEASURE_SPEC_MODE_SHIFT;
    public static final int MEASURE_SPEC_UNSPECIFIED = 0 << MEASURE_SPEC_MODE_SHIFT;
    public static final int MEASURE_SPEC_EXACTLY = 1 << MEASURE_SPEC_MODE_SHIFT;
    public static final int MEASURE_SPEC_AT_MOST = 2 << MEASURE_SPEC_MODE_SHIFT;

    public static final int MATCH_PARENT = -1;
    public static final int WRAP_CONTENT = -2;

    public static int makeMeasureSpec(int size, int mode) {
        return (size & ~MEASURE_SPEC_MODE_MASK) | (mode & MEASURE_SPEC_MODE_MASK);
    }

    public static int getMeasureSpecMode(int measureSpec) {
        return measureSpec & MEASURE_SPEC_MODE_MASK;
    }

    public static int getMeasureSpecSize(int measureSpec) {
        return measureSpec & ~MEASURE_SPEC_MODE_MASK;
    }

    public static class Rect {
        public int left, top, right, bottom;

        public Rect() {}
        public Rect(int l, int t, int r, int b) {
            this.left = l; this.top = t; this.right = r; this.bottom = b;
        }
        public int width() { return Math.max(0, right - left); }
        public int height() { return Math.max(0, bottom - top); }
        public boolean contains(int x, int y) {
            return x >= left && x < right && y >= top && y < bottom;
        }
    }

    public static class JavaViewNode {
        public String id;
        public String type = "ViewGroup";
        public String text;
        public float textSize = 14.0f;
        public Color textColor = new Color(33, 33, 33);
        public Color backgroundColor = new Color(0, 0, 0, 0);
        public Color borderColor;
        public int borderWidth = 0;
        public int cornerRadius = 0;
        public int elevation = 0;
        public int[] padding = new int[4]; // [L, T, R, B]
        public int[] margin = new int[4];  // [L, T, R, B]
        public int layoutWidth = MATCH_PARENT;
        public int layoutHeight = WRAP_CONTENT;
        public float weight = 0.0f;
        public String orientation = "vertical";
        public String gravity = "start";
        public boolean clickable = false;
        public List<JavaViewNode> children = new ArrayList<>();

        public int measuredWidth = 0;
        public int measuredHeight = 0;
        public Rect bounds = new Rect();

        public void measure(int widthMeasureSpec, int heightMeasureSpec) {
            int wMode = getMeasureSpecMode(widthMeasureSpec);
            int wSize = getMeasureSpecSize(widthMeasureSpec);
            int hMode = getMeasureSpecMode(heightMeasureSpec);
            int hSize = getMeasureSpecSize(heightMeasureSpec);

            int padH = padding[0] + padding[2];
            int padV = padding[1] + padding[3];

            boolean isContainer = !children.isEmpty() || isContainerType(type);

            if (!isContainer) {
                int intrinsicW = padH;
                int intrinsicH = padV;

                if (text != null && !text.isEmpty()) {
                    intrinsicW += (int) Math.ceil(text.length() * (textSize * 0.62f));
                    intrinsicH += (int) Math.ceil(textSize * 1.35f);
                } else {
                    intrinsicW += (int) (textSize * 2);
                    intrinsicH += (int) (textSize * 2);
                }

                if (isButtonType(type)) {
                    intrinsicW = Math.max(intrinsicW, 88);
                    intrinsicH = Math.max(intrinsicH, 40);
                }

                measuredWidth = resolveSize(intrinsicW, wSize, wMode, layoutWidth);
                measuredHeight = resolveSize(intrinsicH, hSize, hMode, layoutHeight);
                return;
            }

            boolean isVertical = "vertical".equalsIgnoreCase(orientation) || "Column".equalsIgnoreCase(type)
                    || ("LinearLayout".equalsIgnoreCase(type) && !"horizontal".equalsIgnoreCase(orientation));

            if (isVertical) {
                measureVertical(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode);
            } else {
                measureHorizontal(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode);
            }
        }

        private void measureVertical(int wSpec, int hSpec, int padH, int padV, int wSize, int wMode, int hSize, int hMode) {
            int totalChildH = 0;
            int maxChildW = 0;
            float totalWeight = 0.0f;

            for (JavaViewNode child : children) {
                if (child.weight > 0) {
                    totalWeight += child.weight;
                    continue;
                }
                int marginH = child.margin[0] + child.margin[2];
                int marginV = child.margin[1] + child.margin[3];

                int childWS = getChildSpec(wSpec, padH + marginH, child.layoutWidth);
                int childHS = getChildSpec(hSpec, padV + marginV + totalChildH, child.layoutHeight);
                child.measure(childWS, childHS);

                maxChildW = Math.max(maxChildW, child.measuredWidth + marginH);
                totalChildH += child.measuredHeight + marginV;
            }

            if (totalWeight > 0 && hMode == MEASURE_SPEC_EXACTLY) {
                int remainingH = Math.max(0, hSize - padV - totalChildH);
                for (JavaViewNode child : children) {
                    if (child.weight <= 0) continue;
                    int marginH = child.margin[0] + child.margin[2];
                    int marginV = child.margin[1] + child.margin[3];
                    int weightedH = (int) (remainingH * (child.weight / totalWeight));
                    child.measure(getChildSpec(wSpec, padH + marginH, child.layoutWidth), makeMeasureSpec(weightedH, MEASURE_SPEC_EXACTLY));

                    maxChildW = Math.max(maxChildW, child.measuredWidth + marginH);
                    totalChildH += child.measuredHeight + marginV;
                }
            }

            measuredWidth = resolveSize(maxChildW + padH, wSize, wMode, layoutWidth);
            measuredHeight = resolveSize(totalChildH + padV, hSize, hMode, layoutHeight);
        }

        private void measureHorizontal(int wSpec, int hSpec, int padH, int padV, int wSize, int wMode, int hSize, int hMode) {
            int totalChildW = 0;
            int maxChildH = 0;
            float totalWeight = 0.0f;

            for (JavaViewNode child : children) {
                if (child.weight > 0) {
                    totalWeight += child.weight;
                    continue;
                }
                int marginH = child.margin[0] + child.margin[2];
                int marginV = child.margin[1] + child.margin[3];

                int childWS = getChildSpec(wSpec, padH + marginH + totalChildW, child.layoutWidth);
                int childHS = getChildSpec(hSpec, padV + marginV, child.layoutHeight);
                child.measure(childWS, childHS);

                maxChildH = Math.max(maxChildH, child.measuredHeight + marginV);
                totalChildW += child.measuredWidth + marginH;
            }

            if (totalWeight > 0 && wMode == MEASURE_SPEC_EXACTLY) {
                int remainingW = Math.max(0, wSize - padH - totalChildW);
                for (JavaViewNode child : children) {
                    if (child.weight <= 0) continue;
                    int marginH = child.margin[0] + child.margin[2];
                    int marginV = child.margin[1] + child.margin[3];
                    int weightedW = (int) (remainingW * (child.weight / totalWeight));
                    child.measure(makeMeasureSpec(weightedW, MEASURE_SPEC_EXACTLY), getChildSpec(hSpec, padV + marginV, child.layoutHeight));

                    maxChildH = Math.max(maxChildH, child.measuredHeight + marginV);
                    totalChildW += child.measuredWidth + marginH;
                }
            }

            measuredWidth = resolveSize(totalChildW + padH, wSize, wMode, layoutWidth);
            measuredHeight = resolveSize(maxChildH + padV, hSize, hMode, layoutHeight);
        }

        public void layout(int l, int t, int r, int b) {
            this.bounds = new Rect(l, t, r, b);
            if (children.isEmpty()) return;

            int cLeft = l + padding[0];
            int cTop = t + padding[1];
            int cRight = r - padding[2];

            boolean isVertical = "vertical".equalsIgnoreCase(orientation) || "Column".equalsIgnoreCase(type)
                    || ("LinearLayout".equalsIgnoreCase(type) && !"horizontal".equalsIgnoreCase(orientation));

            if (isVertical) {
                int currY = cTop;
                for (JavaViewNode child : children) {
                    int cL = cLeft + child.margin[0];
                    int cT = currY + child.margin[1];
                    int cR = cL + child.measuredWidth;
                    int cB = cT + child.measuredHeight;

                    if ("center".equalsIgnoreCase(child.gravity) || "center_horizontal".equalsIgnoreCase(child.gravity)) {
                        int availW = (cRight - cLeft) - (child.margin[0] + child.margin[2]);
                        cL = cLeft + child.margin[0] + (availW - child.measuredWidth) / 2;
                        cR = cL + child.measuredWidth;
                    }

                    child.layout(cL, cT, cR, cB);
                    currY = cB + child.margin[3];
                }
            } else {
                int currX = cLeft;
                for (JavaViewNode child : children) {
                    int cL = currX + child.margin[0];
                    int cT = cTop + child.margin[1];
                    int cR = cL + child.measuredWidth;
                    int cB = cT + child.measuredHeight;

                    child.layout(cL, cT, cR, cB);
                    currX = cR + child.margin[2];
                }
            }
        }

        public JavaViewNode hitTest(int x, int y) {
            if (!bounds.contains(x, y)) return null;
            for (int i = children.size() - 1; i >= 0; i--) {
                JavaViewNode hit = children.get(i).hitTest(x, y);
                if (hit != null) return hit;
            }
            return this;
        }

        public void draw(Graphics2D g) {
            int w = bounds.width();
            int h = bounds.height();
            if (w <= 0 || h <= 0) return;

            // Draw elevation shadow
            if (elevation > 0) {
                g.setColor(new Color(0, 0, 0, Math.min(60, elevation * 10)));
                g.fill(new RoundRectangle2D.Float(bounds.left + 2, bounds.top + elevation, w, h, cornerRadius * 2, cornerRadius * 2));
            }

            // Draw background
            if (backgroundColor.getAlpha() > 0) {
                g.setColor(backgroundColor);
                if (cornerRadius > 0) {
                    g.fill(new RoundRectangle2D.Float(bounds.left, bounds.top, w, h, cornerRadius * 2, cornerRadius * 2));
                } else {
                    g.fillRect(bounds.left, bounds.top, w, h);
                }
            }

            // Draw border
            if (borderColor != null && borderWidth > 0) {
                g.setColor(borderColor);
                g.setStroke(new BasicStroke(borderWidth));
                if (cornerRadius > 0) {
                    g.draw(new RoundRectangle2D.Float(bounds.left, bounds.top, w - borderWidth, h - borderWidth, cornerRadius * 2, cornerRadius * 2));
                } else {
                    g.drawRect(bounds.left, bounds.top, w - borderWidth, h - borderWidth);
                }
            }

            // Draw text
            if (text != null && !text.isEmpty()) {
                g.setColor(textColor);
                g.setFont(new Font("SansSerif", Font.PLAIN, (int) textSize));
                FontMetrics fm = g.getFontMetrics();
                int textX = bounds.left + padding[0];
                int textY = bounds.top + padding[1] + fm.getAscent();

                if ("center".equalsIgnoreCase(gravity) || isButtonType(type)) {
                    textX = bounds.left + (w - fm.stringWidth(text)) / 2;
                    textY = bounds.top + (h - fm.getHeight()) / 2 + fm.getAscent();
                }
                g.drawString(text, textX, textY);
            }

            // Draw children
            for (JavaViewNode child : children) {
                child.draw(g);
            }
        }

        private static int resolveSize(int size, int specSize, int specMode, int param) {
            if (specMode == MEASURE_SPEC_EXACTLY) return specSize;
            if (specMode == MEASURE_SPEC_AT_MOST) {
                return (param == MATCH_PARENT) ? specSize : Math.min(size, specSize);
            }
            return (param > 0) ? param : size;
        }

        private static int getChildSpec(int parentSpec, int padding, int childDim) {
            int pMode = getMeasureSpecMode(parentSpec);
            int pSize = Math.max(0, getMeasureSpecSize(parentSpec) - padding);

            if (pMode == MEASURE_SPEC_EXACTLY) {
                if (childDim >= 0) return makeMeasureSpec(childDim, MEASURE_SPEC_EXACTLY);
                if (childDim == MATCH_PARENT) return makeMeasureSpec(pSize, MEASURE_SPEC_EXACTLY);
                return makeMeasureSpec(pSize, MEASURE_SPEC_AT_MOST);
            } else if (pMode == MEASURE_SPEC_AT_MOST) {
                if (childDim >= 0) return makeMeasureSpec(childDim, MEASURE_SPEC_EXACTLY);
                return makeMeasureSpec(pSize, MEASURE_SPEC_AT_MOST);
            }
            return (childDim >= 0) ? makeMeasureSpec(childDim, MEASURE_SPEC_EXACTLY) : makeMeasureSpec(0, MEASURE_SPEC_UNSPECIFIED);
        }

        private static boolean isContainerType(String t) {
            if (t == null) return false;
            String lower = t.toLowerCase();
            return lower.contains("layout") || lower.contains("viewgroup") || lower.contains("card")
                    || lower.contains("column") || lower.contains("row") || lower.contains("box");
        }

        private static boolean isButtonType(String t) {
            return t != null && t.toLowerCase().contains("button");
        }
    }

    /**
     * Executes headless layout and rendering to a BufferedImage.
     */
    public BufferedImage render(JavaViewNode root, int width, int height) {
        int wSpec = makeMeasureSpec(width, MEASURE_SPEC_EXACTLY);
        int hSpec = makeMeasureSpec(height, MEASURE_SPEC_EXACTLY);

        root.measure(wSpec, hSpec);
        root.layout(0, 0, width, height);

        BufferedImage img = new BufferedImage(width, height, BufferedImage.TYPE_INT_ARGB);
        Graphics2D g = img.createGraphics();
        try {
            g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g.setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON);
            root.draw(g);
        } finally {
            g.dispose();
        }
        return img;
    }

    public static void main(String[] args) throws Exception {
        int width = 540;
        int height = 1200;
        if (args.length >= 2) {
            width = Integer.parseInt(args[0]);
            height = Integer.parseInt(args[1]);
        }

        JavaViewNode root = new JavaViewNode();
        root.type = "LinearLayout";
        root.orientation = "vertical";
        root.backgroundColor = new Color(245, 245, 247);
        root.padding = new int[]{24, 48, 24, 24};

        JavaViewNode card = new JavaViewNode();
        card.type = "CardView";
        card.backgroundColor = Color.WHITE;
        card.cornerRadius = 16;
        card.elevation = 6;
        card.padding = new int[]{20, 20, 20, 20};
        card.margin = new int[]{0, 0, 0, 16};

        JavaViewNode title = new JavaViewNode();
        title.type = "TextView";
        title.text = "Nano-Android Headless Layout";
        title.textSize = 20.0f;
        title.textColor = new Color(17, 24, 39);
        card.children.add(title);

        JavaViewNode button = new JavaViewNode();
        button.type = "Button";
        button.text = "Click Me";
        button.textSize = 16.0f;
        button.backgroundColor = new Color(37, 99, 235);
        button.textColor = Color.WHITE;
        button.cornerRadius = 8;
        button.elevation = 4;
        button.padding = new int[]{16, 12, 16, 12};

        root.children.add(card);
        root.children.add(button);

        LayoutRenderer renderer = new LayoutRenderer();
        BufferedImage img = renderer.render(root, width, height);

        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        ImageIO.write(img, "png", baos);
        String base64 = Base64.getEncoder().encodeToString(baos.toByteArray());

        System.out.println("{\"status\":\"ok\",\"width\":" + width + ",\"height\":" + height + ",\"data_uri\":\"data:image/png;base64," + base64 + "\"}");
    }
}
