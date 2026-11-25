package android.content.res;

import org.xmlpull.v1.XmlPullParser;
import android.util.AttributeSet;

public interface XmlResourceParser extends XmlPullParser, AttributeSet, AutoCloseable {
    String getAttributeNamespace(int index);
    void close();
}
