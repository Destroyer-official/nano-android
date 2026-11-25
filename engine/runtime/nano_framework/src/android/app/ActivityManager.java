package android.app;

public class ActivityManager {
    public static class MemoryInfo {
        public long availMem = 4L * 1024 * 1024 * 1024;
        public long totalMem = 8L * 1024 * 1024 * 1024;
        public boolean lowMemory = false;
    }
    public void getMemoryInfo(MemoryInfo outInfo) {}
    public int getMemoryClass() { return 256; }
    public int getLargeMemoryClass() { return 512; }
    public boolean isLowRamDevice() { return false; }

    public static class RunningAppProcessInfo {
        public String processName = "com.example.app";
        public int pid = 1088;
        public int uid = 10088;
        public String[] pkgList = new String[]{"com.example.app"};
        public int importance = 100;
    }

    public java.util.List<RunningAppProcessInfo> getRunningAppProcesses() {
        java.util.List<RunningAppProcessInfo> list = new java.util.ArrayList<>();
        list.add(new RunningAppProcessInfo());
        return list;
    }

    public static class TaskDescription implements android.os.Parcelable {
        private String label;
        private android.graphics.Bitmap icon;
        private int primaryColor;

        public TaskDescription() {}
        public TaskDescription(String label) { this.label = label; }
        public TaskDescription(String label, int color) { this.label = label; this.primaryColor = color; }
        public TaskDescription(String label, int iconRes, int color) { this.label = label; this.primaryColor = color; }
        public TaskDescription(String label, android.graphics.Bitmap icon, int color) {
            this.label = label; this.icon = icon; this.primaryColor = color;
        }
        public TaskDescription(String label, android.graphics.Bitmap icon) {
            this.label = label; this.icon = icon;
        }
        public TaskDescription(TaskDescription other) {
            if (other != null) {
                this.label = other.label;
                this.icon = other.icon;
                this.primaryColor = other.primaryColor;
            }
        }

        public String getLabel() { return label; }
        public android.graphics.Bitmap getIcon() { return icon; }
        public int getPrimaryColor() { return primaryColor; }
        public int getBackgroundColor() { return 0; }
        public int getStatusBarColor() { return 0; }
        public int getNavigationBarColor() { return 0; }
        @Override public int describeContents() { return 0; }
        @Override public void writeToParcel(android.os.Parcel dest, int flags) {}
    }
}
