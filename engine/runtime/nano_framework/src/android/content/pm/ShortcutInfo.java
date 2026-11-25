package android.content.pm;

import android.content.Context;
import android.content.Intent;
import android.graphics.drawable.Icon;

public final class ShortcutInfo {
    private final String id;
    private CharSequence shortLabel;
    private CharSequence longLabel;
    private Icon icon;
    private Intent intent;

    private ShortcutInfo(Builder builder) {
        this.id = builder.id;
        this.shortLabel = builder.shortLabel;
        this.longLabel = builder.longLabel;
        this.icon = builder.icon;
        this.intent = builder.intent;
    }

    public String getId() { return id; }
    public CharSequence getShortLabel() { return shortLabel; }
    public CharSequence getLongLabel() { return longLabel; }
    public Intent getIntent() { return intent; }

    public static class Builder {
        final Context context;
        final String id;
        CharSequence shortLabel = "";
        CharSequence longLabel = "";
        Icon icon;
        Intent intent;

        public Builder(Context context, String id) {
            this.context = context;
            this.id = id;
        }

        public Builder setShortLabel(CharSequence shortLabel) {
            this.shortLabel = shortLabel;
            return this;
        }

        public Builder setLongLabel(CharSequence longLabel) {
            this.longLabel = longLabel;
            return this;
        }

        public Builder setIcon(Icon icon) {
            this.icon = icon;
            return this;
        }

        public Builder setIntent(Intent intent) {
            this.intent = intent;
            return this;
        }

        public ShortcutInfo build() {
            return new ShortcutInfo(this);
        }
    }
}
