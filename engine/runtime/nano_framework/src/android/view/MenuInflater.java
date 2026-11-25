package android.view;

import android.content.Context;

public class MenuInflater {
    private final Context context;

    public MenuInflater(Context context) {
        this.context = context;
    }

    public void inflate(int menuRes, Menu menu) {
        if (menu != null) {
            menu.add(0, menuRes, 0, "Item " + menuRes);
        }
    }
}
