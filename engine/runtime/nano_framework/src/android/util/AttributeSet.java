package android.util;

public interface AttributeSet {
    int getAttributeCount();
    default String getAttributeNamespace(int index) { return ""; }
    String getAttributeName(int index);
    String getAttributeValue(int index);
    String getAttributeValue(String namespace, String name);
    String getPositionDescription();
    default int getAttributeNameResource(int index) { return 0; }
    default int getAttributeListValue(String namespace, String attribute, String[] options, int defaultValue) { return defaultValue; }
    default boolean getAttributeBooleanValue(String namespace, String attribute, boolean defaultValue) { return defaultValue; }
    default int getAttributeResourceValue(String namespace, String attribute, int defaultValue) { return defaultValue; }
    default int getAttributeIntValue(String namespace, String attribute, int defaultValue) { return defaultValue; }
    default int getAttributeUnsignedIntValue(String namespace, String attribute, int defaultValue) { return defaultValue; }
    default float getAttributeFloatValue(String namespace, String attribute, float defaultValue) { return defaultValue; }
    default int getAttributeListValue(int index, String[] options, int defaultValue) { return defaultValue; }
    default boolean getAttributeBooleanValue(int index, boolean defaultValue) { return defaultValue; }
    default int getAttributeResourceValue(int index, int defaultValue) { return defaultValue; }
    default int getAttributeIntValue(int index, int defaultValue) { return defaultValue; }
    default int getAttributeUnsignedIntValue(int index, int defaultValue) { return defaultValue; }
    default float getAttributeFloatValue(int index, float defaultValue) { return defaultValue; }
    default String getIdAttribute() { return null; }
    default String getClassAttribute() { return null; }
    default int getIdAttributeResourceValue(int defaultValue) { return defaultValue; }
    default int getStyleAttribute() { return 0; }
}
