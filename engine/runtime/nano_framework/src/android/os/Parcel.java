package android.os;

public class Parcel {
    public static Parcel obtain() { return new Parcel(); }
    public void recycle() {}
    public void writeInt(int val) {}
    public int readInt() { return 0; }
    public void writeString(String val) {}
    public String readString() { return null; }
    public void writeByte(byte val) {}
    public byte readByte() { return 0; }
}
