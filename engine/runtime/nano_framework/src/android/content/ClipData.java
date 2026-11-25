package android.content;

import android.net.Uri;
import java.util.ArrayList;
import java.util.List;

public class ClipData {
    public static class Item {
        private final CharSequence text;
        private final Uri uri;
        private final Intent intent;

        public Item(CharSequence text) { this.text = text; this.uri = null; this.intent = null; }
        public Item(Uri uri) { this.text = null; this.uri = uri; this.intent = null; }
        public Item(Intent intent) { this.text = null; this.uri = null; this.intent = intent; }

        public CharSequence getText() { return text; }
        public Uri getUri() { return uri; }
        public Intent getIntent() { return intent; }
    }

    private final List<Item> items = new ArrayList<>();

    public ClipData() {}

    public ClipData(CharSequence label, String[] mimeTypes, Item item) {
        if (item != null) items.add(item);
    }

    public static ClipData newPlainText(CharSequence label, CharSequence text) {
        return new ClipData(label, new String[]{"text/plain"}, new Item(text));
    }

    public static ClipData newUri(ContentResolver resolver, CharSequence label, Uri uri) {
        return new ClipData(label, new String[]{"*/*"}, new Item(uri));
    }

    public static ClipData newRawUri(CharSequence label, Uri uri) {
        return new ClipData(label, new String[]{"*/*"}, new Item(uri));
    }

    public void addItem(Item item) {
        if (item != null) items.add(item);
    }

    public int getItemCount() {
        return items.size();
    }

    public Item getItemAt(int index) {
        return items.get(index);
    }
}
