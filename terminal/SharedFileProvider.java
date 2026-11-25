package dev.galaxy.terminal;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.system.ErrnoException;
import android.system.Os;
import android.system.OsConstants;
import java.io.FileDescriptor;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.file.DirectoryStream;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.Path;
import java.util.List;
import java.util.concurrent.TimeUnit;

public final class SharedFileProvider extends ContentProvider {
    private static final String CACHE = "galaxy-shared";

    static Uri stage(Context context, Path source, String name) throws IOException {
        Path cache = Files.createDirectories(context.getCacheDir().toPath().resolve(CACHE));
        clean(cache);
        Path slot = Files.createTempDirectory(cache, "share-");
        Path attachment = slot.resolve(name);
        try {
            FileDescriptor descriptor = Os.open(source.toString(),
                    OsConstants.O_RDONLY | OsConstants.O_NOFOLLOW | OsConstants.O_NONBLOCK, 0);
            ParcelFileDescriptor owned;
            try {
                if (!OsConstants.S_ISREG(Os.fstat(descriptor).st_mode)) throw new IOException("일반 파일만 공유할 수 있습니다.");
                owned = ParcelFileDescriptor.dup(descriptor);
            } finally {
                Os.close(descriptor);
            }
            try (ParcelFileDescriptor.AutoCloseInputStream input = new ParcelFileDescriptor.AutoCloseInputStream(owned)) {
                Files.copy(input, attachment);
            }
        } catch (ErrnoException | IOException error) {
            Files.deleteIfExists(attachment);
            Files.deleteIfExists(slot);
            throw new IOException(error);
        }
        return new Uri.Builder().scheme("content").authority(context.getPackageName() + ".shared")
                .appendPath(slot.getFileName().toString()).appendPath(name).build();
    }

    private static void clean(Path cache) {
        long stale = System.currentTimeMillis() - TimeUnit.DAYS.toMillis(1);
        try (DirectoryStream<Path> slots = Files.newDirectoryStream(cache, "share-*")) {
            for (Path slot : slots) {
                if (!Files.isDirectory(slot, LinkOption.NOFOLLOW_LINKS)
                        || Files.getLastModifiedTime(slot, LinkOption.NOFOLLOW_LINKS).toMillis() >= stale) continue;
                try (DirectoryStream<Path> files = Files.newDirectoryStream(slot)) {
                    for (Path file : files) Files.deleteIfExists(file);
                }
                Files.deleteIfExists(slot);
            }
        } catch (IOException ignored) {
            // Cache cleanup is independent of preparing the next attachment.
        }
    }

    private Path attachment(Uri uri) throws FileNotFoundException {
        List<String> parts = uri.getPathSegments();
        if (parts.size() != 2 || !parts.get(0).matches("share-[0-9]+") || !FileTransfer.safeName(parts.get(1))) {
            throw new FileNotFoundException();
        }
        Path path = getContext().getCacheDir().toPath().resolve(CACHE).resolve(parts.get(0)).resolve(parts.get(1));
        if (!Files.isRegularFile(path, LinkOption.NOFOLLOW_LINKS)) throw new FileNotFoundException();
        return path;
    }

    @Override public boolean onCreate() { return true; }

    @Override public String getType(Uri uri) {
        try { return FileTransfer.mimeType(attachment(uri).getFileName().toString()); }
        catch (FileNotFoundException error) { return null; }
    }

    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
        String[] columns = projection != null ? projection : new String[] { OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE };
        MatrixCursor cursor = new MatrixCursor(columns, 1);
        try {
            Path file = attachment(uri);
            Object[] values = new Object[columns.length];
            for (int i = 0; i < columns.length; i++) {
                if (OpenableColumns.DISPLAY_NAME.equals(columns[i])) values[i] = file.getFileName().toString();
                else if (OpenableColumns.SIZE.equals(columns[i])) values[i] = Files.size(file);
            }
            cursor.addRow(values);
        } catch (IOException ignored) { }
        return cursor;
    }

    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) throw new FileNotFoundException("읽기 전용 파일입니다.");
        return ParcelFileDescriptor.open(attachment(uri).toFile(), ParcelFileDescriptor.MODE_READ_ONLY);
    }

    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException(); }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] args) { throw new UnsupportedOperationException(); }
    @Override public int delete(Uri uri, String selection, String[] args) { throw new UnsupportedOperationException(); }
}
