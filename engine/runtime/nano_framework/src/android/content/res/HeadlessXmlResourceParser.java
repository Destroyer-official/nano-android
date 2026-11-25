package android.content.res;

import org.xmlpull.v1.XmlPullParser;
import org.xmlpull.v1.XmlPullParserException;
import java.io.IOException;

public class HeadlessXmlResourceParser implements XmlResourceParser {
    private int state = 0; // 0: START_DOCUMENT, 1: START_TAG, 2: END_TAG, 3: END_DOCUMENT

    @Override
    public int getEventType() {
        switch (state) {
            case 0: return START_DOCUMENT;
            case 1: return START_TAG;
            case 2: return END_TAG;
            default: return END_DOCUMENT;
        }
    }

    @Override
    public int next() throws IOException, XmlPullParserException {
        if (state < 3) state++;
        return getEventType();
    }

    @Override
    public int nextToken() throws IOException, XmlPullParserException {
        return next();
    }

    @Override
    public int nextTag() throws IOException, XmlPullParserException {
        return next();
    }

    @Override
    public String getName() {
        return "menu";
    }

    @Override
    public String getAttributeNamespace(int index) { return ""; }
    @Override
    public void close() {}

    @Override
    public void setFeature(String name, boolean state) {}
    @Override
    public boolean getFeature(String name) { return false; }
    @Override
    public void setProperty(String name, Object value) {}
    @Override
    public Object getProperty(String name) { return null; }
    @Override
    public void setInput(java.io.Reader in) {}
    @Override
    public void setInput(java.io.InputStream in, String inputEncoding) {}
    @Override
    public String getInputEncoding() { return "UTF-8"; }
    @Override
    public void defineEntityReplacementText(String entityName, String replacementText) {}
    @Override
    public int getNamespaceCount(int depth) { return 0; }
    @Override
    public String getNamespacePrefix(int pos) { return null; }
    @Override
    public String getNamespaceUri(int pos) { return null; }
    @Override
    public String getNamespace(String prefix) { return null; }
    @Override
    public int getDepth() { return state == 1 ? 1 : 0; }
    @Override
    public String getPositionDescription() { return ""; }
    @Override
    public int getLineNumber() { return 1; }
    @Override
    public int getColumnNumber() { return 1; }
    @Override
    public boolean isWhitespace() { return false; }
    @Override
    public String getText() { return null; }
    @Override
    public char[] getTextCharacters(int[] holderForStartAndLength) { return new char[0]; }
    @Override
    public String getNamespace() { return ""; }
    @Override
    public String getPrefix() { return null; }
    @Override
    public boolean isEmptyElementTag() { return false; }
    @Override
    public int getAttributeCount() { return 0; }
    @Override
    public String getAttributeName(int index) { return ""; }
    @Override
    public String getAttributePrefix(int index) { return null; }
    @Override
    public String getAttributeType(int index) { return "CDATA"; }
    @Override
    public boolean isAttributeDefault(int index) { return false; }
    @Override
    public String getAttributeValue(int index) { return null; }
    @Override
    public String getAttributeValue(String namespace, String name) { return null; }
    @Override
    public void require(int type, String namespace, String name) {}
    @Override
    public String nextText() { return ""; }
}
