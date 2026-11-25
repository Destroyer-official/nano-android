package android.os;

import java.io.*;

public class ParcelFileDescriptor implements Closeable {
    public static final int MODE_READ_ONLY = 0x10000000;
    public static final int MODE_WRITE_ONLY = 0x20000000;
    public static final int MODE_READ_WRITE = 0x30000000;
    public static final int MODE_CREATE = 0x08000000;
    public static final int MODE_TRUNCATE = 0x04000000;

    private final File file;
    private FileDescriptor fd;
    private RandomAccessFile raf;

    private ParcelFileDescriptor(File file, int mode) throws FileNotFoundException {
        this.file = file;
        String rafMode = ((mode & MODE_WRITE_ONLY) != 0 || (mode & MODE_READ_WRITE) == MODE_READ_WRITE) ? "rw" : "r";
        if (file != null && file.exists()) {
            try {
                this.raf = new RandomAccessFile(file, rafMode);
                this.fd = this.raf.getFD();
            } catch (IOException e) {
                throw new FileNotFoundException(e.getMessage());
            }
        }
    }

    public static ParcelFileDescriptor open(File file, int mode) throws FileNotFoundException {
        if (file == null || !file.exists()) {
            throw new FileNotFoundException("File not found: " + file);
        }
        return new ParcelFileDescriptor(file, mode);
    }

    public FileDescriptor getFileDescriptor() {
        return fd != null ? fd : FileDescriptor.in;
    }

    public long getStatSize() {
        return file != null && file.exists() ? file.length() : -1L;
    }

    @Override
    public void close() throws IOException {
        if (raf != null) raf.close();
    }

    public static class AutoCloseInputStream extends FileInputStream {
        private final ParcelFileDescriptor pfd;
        public AutoCloseInputStream(ParcelFileDescriptor pfd) throws FileNotFoundException {
            super(pfd.file);
            this.pfd = pfd;
        }
        @Override
        public void close() throws IOException {
            try { super.close(); } finally { if (pfd != null) pfd.close(); }
        }
    }

    public static class AutoCloseOutputStream extends FileOutputStream {
        private final ParcelFileDescriptor pfd;
        public AutoCloseOutputStream(ParcelFileDescriptor pfd) throws FileNotFoundException {
            super(pfd.file);
            this.pfd = pfd;
        }
        @Override
        public void close() throws IOException {
            try { super.close(); } finally { if (pfd != null) pfd.close(); }
        }
    }
}
