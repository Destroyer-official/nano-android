package android.graphics.drawable;

import java.util.HashMap;
import java.util.Map;

public class LayerDrawable extends Drawable {
    private final Drawable[] layers;
    private final Map<Integer, Drawable> layerMap = new HashMap<>();

    public LayerDrawable(Drawable[] layers) {
        this.layers = layers != null ? layers : new Drawable[0];
    }

    public Drawable getDrawable(int index) {
        if (layers != null && index >= 0 && index < layers.length) {
            return layers[index];
        }
        return new ColorDrawable(0xFF888888);
    }

    public int getNumberOfLayers() {
        return layers != null ? layers.length : 0;
    }

    public boolean setDrawableByLayerId(int id, Drawable drawable) {
        layerMap.put(id, drawable);
        return true;
    }

    public Drawable findDrawableByLayerId(int id) {
        Drawable d = layerMap.get(id);
        if (d == null) {
            d = new ColorDrawable(0xFF888888);
            layerMap.put(id, d);
        }
        return d;
    }
}
