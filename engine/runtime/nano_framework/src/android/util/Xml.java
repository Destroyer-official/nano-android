package android.util;

import org.xmlpull.v1.XmlPullParser;

public class Xml {
    public static String FEATURE_RELAXED = "http://xmlpull.org/v1/doc/features.html#relaxed";

    public enum Encoding {
        US_ASCII("US-ASCII"),
        UTF_8("UTF-8"),
        UTF_16("UTF-16"),
        ISO_8859_1("ISO-8859-1");

        final String exp;
        Encoding(String exp) {
            this.exp = exp;
        }
    }

    public static AttributeSet asAttributeSet(XmlPullParser parser) {
        if (parser instanceof AttributeSet) {
            return (AttributeSet) parser;
        }
        return new AttributeSet() {
            @Override public int getAttributeCount() { return parser.getAttributeCount(); }
            @Override public String getAttributeName(int index) { return parser.getAttributeName(index); }
            @Override public String getAttributeValue(int index) { return parser.getAttributeValue(index); }
            @Override public String getAttributeValue(String namespace, String name) { return parser.getAttributeValue(namespace, name); }
            @Override public String getPositionDescription() { return parser.getPositionDescription(); }
        };
    }
}
