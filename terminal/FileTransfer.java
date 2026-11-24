package dev.galaxy.terminal;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.file.DirectoryStream;
import java.nio.file.FileAlreadyExistsException;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.function.Consumer;

final class FileTransfer {
    private static final int IMPORT = 401;
    private static final int EXPORT = 402;
    private static final String EXPORT_NAME = "galaxy.transfer.export.name";

    private final Activity activity;
    private final Consumer<String> notice;
    private String exportName;
    private boolean transferring;

    FileTransfer(Activity activity, Consumer<String> notice, Bundle state) {
        this.activity = activity;
        this.notice = notice;
        if (state != null) exportName = state.getString(EXPORT_NAME);
    }

    static Path directory(Context context) throws IOException {
        Path path = context.getNoBackupFilesDir().toPath().resolve("galaxy-transfer");
        try {
            return Files.createDirectory(path);
        } catch (FileAlreadyExistsException exists) {
            if (Files.isDirectory(path, LinkOption.NOFOLLOW_LINKS)) return path;
            throw exists;
        }
    }

    void save(Bundle state) {
        state.putString(EXPORT_NAME, exportName);
    }

    void show() {
        if (transferring) {
            notice.accept("파일을 전송하는 중입니다.");
            return;
        }
        new AlertDialog.Builder(activity).setTitle("Android 파일 전송")
                .setItems(new String[] { "Android에서 가져오기", "Android에 저장하기" }, (dialog, choice) -> {
                    if (choice == 0) {
                        launch(new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*"), IMPORT);
                    } else {
                        chooseExport();
                    }
                }).show();
    }

    private void chooseExport() {
        List<String> names = new ArrayList<>();
        try (DirectoryStream<Path> entries = Files.newDirectoryStream(directory(activity))) {
            for (Path entry : entries) {
                String name = entry.getFileName().toString();
                if (safeName(name) && !name.startsWith(".galaxy-")
                        && Files.isRegularFile(entry, LinkOption.NOFOLLOW_LINKS)) {
                    names.add(name);
                }
            }
        } catch (IOException | SecurityException error) {
            notice.accept("전송 폴더를 열 수 없습니다.");
            return;
        }
        if (names.isEmpty()) {
            notice.accept("보낼 파일을 /mnt/galaxy-transfer에 복사하세요.");
            return;
        }
        Collections.sort(names);
        new AlertDialog.Builder(activity).setTitle("저장할 파일")
                .setItems(names.toArray(new String[0]), (dialog, choice) -> {
                    exportName = names.get(choice);
                    String type = mimeType(exportName);
                    Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT)
                            .setType(type).putExtra(Intent.EXTRA_TITLE, exportName);
                    if (!launch(intent, EXPORT)) exportName = null;
                }).show();
    }

    private boolean launch(Intent intent, int request) {
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        try {
            activity.startActivityForResult(intent, request);
            return true;
        } catch (ActivityNotFoundException | SecurityException unavailable) {
            notice.accept("Android 파일 선택기를 열 수 없습니다.");
            return false;
        }
    }

    void result(int request, int result, Intent data) {
        if (request != IMPORT && request != EXPORT) return;
        String name = exportName;
        exportName = null;
        if (result != Activity.RESULT_OK || data == null || data.getData() == null) return;
        Uri uri = data.getData();
        if (!"content".equals(uri.getScheme())) {
            notice.accept("파일 선택기에서 파일을 선택하세요.");
            return;
        }
        transferring = true;
        notice.accept("파일 전송 중…");
        new Thread(() -> {
            String message;
            try {
                message = request == IMPORT ? importFile(uri) : exportFile(uri, name);
            } catch (IOException | RuntimeException error) {
                message = "파일 전송에 실패했습니다.";
            }
            String resultMessage = message;
            activity.runOnUiThread(() -> {
                transferring = false;
                if (!activity.isFinishing() && !activity.isDestroyed()) notice.accept(resultMessage);
            });
        }, "galaxy-file-transfer").start();
    }

    private String importFile(Uri uri) throws IOException {
        String name = "Android-file";
        try (Cursor cursor = activity.getContentResolver().query(uri,
                new String[] { OpenableColumns.DISPLAY_NAME }, null, null, null)) {
            if (cursor != null && cursor.moveToFirst() && safeName(cursor.getString(0))) {
                name = cursor.getString(0);
            }
        }
        if (name.startsWith(".galaxy-")) name = "Android-" + name;
        Path dir = directory(activity);
        Path stage = Files.createTempFile(dir, ".galaxy-import-", ".tmp");
        try {
            try (InputStream in = activity.getContentResolver().openInputStream(uri);
                 OutputStream out = Files.newOutputStream(stage)) {
                if (in == null) throw new IOException("파일 읽기 실패");
                copy(in, out);
            }
            int dot = name.lastIndexOf('.');
            String stem = dot > 0 ? name.substring(0, dot) : name;
            String extension = dot > 0 ? name.substring(dot) : "";
            for (int attempt = 0; attempt < 1000; attempt++) {
                String candidate = attempt == 0 ? name : stem + "-" + attempt + extension;
                try {
                    Files.move(stage, dir.resolve(candidate));
                    return "가져옴 · /mnt/galaxy-transfer/" + candidate;
                } catch (FileAlreadyExistsException exists) {
                    // Try another name without overwriting an existing file.
                }
            }
            throw new IOException("빈 파일 이름을 찾을 수 없습니다.");
        } finally {
            Files.deleteIfExists(stage);
        }
    }

    private String exportFile(Uri uri, String name) throws IOException {
        if (!safeName(name)) throw new IOException("파일 이름 없음");
        Path source = directory(activity).resolve(name);
        if (!Files.isRegularFile(source, LinkOption.NOFOLLOW_LINKS)) throw new IOException("파일 없음");
        try (InputStream in = Files.newInputStream(source, LinkOption.NOFOLLOW_LINKS);
             OutputStream out = activity.getContentResolver().openOutputStream(uri)) {
            if (out == null) throw new IOException("파일 쓰기 실패");
            copy(in, out);
        }
        return "Android에 저장함 · " + name;
    }

    private static boolean safeName(String name) {
        if (name == null || name.isEmpty() || name.equals(".") || name.equals("..")
                || name.indexOf('/') >= 0 || name.indexOf('\\') >= 0) return false;
        for (int i = 0; i < name.length(); i++) {
            if (Character.isISOControl(name.charAt(i))) return false;
        }
        return true;
    }

    private static void copy(InputStream in, OutputStream out) throws IOException {
        byte[] buffer = new byte[64 * 1024];
        int count;
        while ((count = in.read(buffer)) != -1) out.write(buffer, 0, count);
    }

    private static String mimeType(String name) {
        int dot = name.lastIndexOf('.');
        if (dot >= 0 && dot + 1 < name.length()) {
            String extension = name.substring(dot + 1).toLowerCase(Locale.ROOT);
            String mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension);
            if (mime != null) return mime;
        }
        return "application/octet-stream";
    }
}
