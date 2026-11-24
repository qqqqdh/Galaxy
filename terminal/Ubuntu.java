package dev.galaxy.terminal;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.os.Build;
import android.os.Process;
import android.util.Log;
import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.RandomAccessFile;
import java.net.HttpURLConnection;
import java.net.InetAddress;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.FileAlreadyExistsException;
import java.nio.file.FileVisitResult;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.SimpleFileVisitor;
import java.nio.file.StandardCopyOption;
import java.nio.file.attribute.BasicFileAttributes;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Properties;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.locks.ReentrantLock;
import java.util.function.Consumer;

final class Ubuntu {
    private static final String IMAGE_URL = "https://cdimage.ubuntu.com/ubuntu-base/releases/24.04.5/release/"
            + "ubuntu-base-24.04.5-base-arm64.tar.gz";
    private static final long IMAGE_SIZE = 29_936_675;
    private static final String IMAGE_SHA256 = "a91d5a93010193712d346d761372b7c9db6dfcf093893161c64ca107f05914f2";
    // Keep this list in sync with the hard links in the verified Ubuntu image.
    private static final String[][] IMAGE_HARD_LINKS = {
        { "usr/bin/perl5.38.2", "perl" },
        { "usr/bin/uncompress", "gunzip" }
    };
    private static final ReentrantLock INSTALL_LOCK = new ReentrantLock();

    static boolean supported(Context context) {
        return Build.VERSION.SDK_INT >= 28 && Process.is64Bit()
                && Arrays.asList(Build.SUPPORTED_64_BIT_ABIS).contains("arm64-v8a")
                && new File(context.getApplicationInfo().nativeLibraryDir, "libproot.so").isFile()
                && new File(context.getApplicationInfo().nativeLibraryDir, "libproot-loader.so").isFile();
    }

    private static Path accountFile(Context context) {
        return context.getNoBackupFilesDir().toPath().resolve("ubuntu-account.properties");
    }

    static boolean configured(Context context) { return Files.isRegularFile(accountFile(context)); }

    static PtySession.Command prepare(Context context, Consumer<String> progress, UbuntuSetup.Options setup)
            throws IOException, InterruptedException {
        try {
            if (!supported(context)) throw new IOException("Ubuntu는 ARM64 Android 9 이상에서 열 수 있습니다.");
            INSTALL_LOCK.lockInterruptibly();
            try {
                Path root = install(context, progress);
                Path temporary = Files.createDirectories(context.getCacheDir().toPath().resolve("ubuntu-runtime"));
                Files.write(temporary.resolve("resolv.conf"), dnsConfiguration(context).getBytes(StandardCharsets.US_ASCII));
                copyAsset(context, "galaxy-bashrc", temporary.resolve("galaxy-bashrc"));
                Path w = Files.createDirectories(temporary.resolve("galaxy-bin")).resolve("w");
                copyAsset(context, "galaxy-w", w);
                if (!w.toFile().setExecutable(true, true)) throw new IOException("w 실행 권한 설정 실패");
                copyAsset(context, "galaxy-ubuntu-setup.sh", temporary.resolve("galaxy-ubuntu-setup.sh"));
                Files.createDirectories(root.resolve(".l2s"));
                Path saved = accountFile(context);
                if (!configured(context)) {
                    if (setup == null) throw new IOException("Ubuntu 초기 설정을 먼저 완료하세요.");
                    configure(context, root, temporary, saved, setup, progress);
                }
                Properties account = new Properties();
                try (InputStream input = Files.newInputStream(saved)) { account.load(input); }
                String user = account.getProperty("user", "");
                String ids = account.getProperty("ids", "");
                String computer = account.getProperty("computer", "");
                String locale = account.getProperty("locale", "");
                if (!UbuntuSetup.validUser(user) || !ids.matches("[1-9][0-9]*:[1-9][0-9]*")
                        || !UbuntuSetup.validComputer(computer) || !UbuntuSetup.locales(context).contains(locale)) {
                    throw new IOException("저장된 Ubuntu 계정 설정을 읽을 수 없습니다.");
                }
                progress.accept("Ubuntu 여는 중");
                return command(context, root, temporary, user, ids, computer, locale,
                        "/bin/bash", "--noprofile", "--rcfile", "/etc/galaxy-bashrc", "-i");
            } finally {
                INSTALL_LOCK.unlock();
            }
        } finally {
            if (setup != null) setup.clearPassword();
        }
    }

    private static PtySession.Command command(Context context, Path root, Path temporary, String user, String ids,
            String computer, String locale, String... guestCommand) throws IOException {
        File libraries = new File(context.getApplicationInfo().nativeLibraryDir);
        Path hardlinks = root.resolve(".l2s");
        String home = user.equals("root") ? "/root" : "/home/" + user;
        String window = Long.toString(android.os.SystemClock.elapsedRealtimeNanos());
        File sessionHelper = new File(libraries, "libgalaxy_session.so");
        List<String> arguments = new ArrayList<>(Arrays.asList(
            new File(libraries, "libproot.so").getPath(),
            "--link2symlink", "--kill-on-exit", "--sysvipc", "-i", ids, "-r", root.toString(),
            "-b", "/dev", "-b", "/proc", "-b", "/sys", "-w", home,
            "-b", "/proc/self/fd:/dev/fd", "-b", root.resolve("tmp") + ":/dev/shm",
            "-b", hardlinks.toString(),
            "-b", temporary.resolve("resolv.conf") + ":/etc/resolv.conf!",
            "-b", temporary.resolve("galaxy-bashrc") + ":/etc/galaxy-bashrc!",
            "-b", temporary.resolve("galaxy-bin") + ":/run/galaxy-bin!",
            "-b", sessionHelper + ":/usr/libexec/galaxy-session!",
            "-b", FileTransfer.directory(context) + ":/mnt/galaxy-transfer!",
            "-b", temporary.resolve("galaxy-ubuntu-setup.sh") + ":/etc/galaxy-ubuntu-setup.sh!",
            "/usr/bin/env", "-i", "HOME=" + home, "USER=" + user, "LOGNAME=" + user, "SHELL=/bin/bash",
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/run/galaxy-bin:/usr/bin:/sbin:/bin", "TMPDIR=/tmp",
            "TERM=xterm-256color", "LANG=" + locale, "GALAXY_HOSTNAME=" + computer, "GALAXY_UTMP_WINDOW=" + window));
        arguments.addAll(Arrays.asList(guestCommand));
        PtySession.Command command = new PtySession.Command(root.toFile(), arguments.toArray(new String[0]),
                "PATH=/system/bin:/system/xbin", "ANDROID_ROOT=/system", "ANDROID_DATA=/data",
                "PROOT_NO_SECCOMP=1", "PROOT_TMP_DIR=" + temporary, "PROOT_L2S_DIR=" + hardlinks,
                "PROOT_LOADER=" + new File(libraries, "libproot-loader.so"));
        command.onExit = () -> {
            // Closing the Android window can stop its guest login helpers along with PRoot.
            java.lang.Process cleanup = new ProcessBuilder(sessionHelper.toString(), "--close-window", window)
                    .directory(root.toFile()).start();
            boolean interrupted = Thread.interrupted();
            try {
                for (;;) {
                    try {
                        if (cleanup.waitFor() != 0) throw new IOException("Ubuntu 세션 정리 실패");
                        break;
                    } catch (InterruptedException retry) { interrupted = true; }
                }
            } finally {
                if (interrupted) Thread.currentThread().interrupt();
            }
        };
        return command;
    }

    private static void configure(Context context, Path root, Path temporary, Path saved, UbuntuSetup.Options setup,
            Consumer<String> progress) throws IOException, InterruptedException {
        progress.accept("Ubuntu 초기 설정 중");
        PtySession.Command provision = command(context, root, temporary, "root", "0:0", setup.computer, "C.UTF-8",
                "/bin/bash", "/etc/galaxy-ubuntu-setup.sh",
                setup.user, setup.fullName, setup.computer, setup.locale, setup.zone);
        List<String> output = runSetup(provision, temporary, setup.user + ":", setup.password, progress);
        String ids = "";
        for (String line : output) if (line.startsWith("GALAXY_ACCOUNT=")) ids = line.substring(15);
        if (!ids.matches("[1-9][0-9]*:[1-9][0-9]*")) throw new IOException("일반 사용자 계정 생성에 실패했습니다.");

        progress.accept("관리자 비밀번호와 sudo 확인 중");
        PtySession.Command check = command(context, root, temporary, setup.user, ids, setup.computer, "C.UTF-8",
                "/usr/bin/sudo", "-S", "-k", "-p", "", "--", "/usr/bin/id", "-u");
        if (!runSetup(check, temporary, "", setup.password, progress).contains("0")) {
            throw new IOException("sudo로 Ubuntu 관리자 권한을 확인할 수 없습니다.");
        }
        Properties account = new Properties();
        account.setProperty("user", setup.user);
        account.setProperty("ids", ids);
        account.setProperty("computer", setup.computer);
        account.setProperty("locale", setup.locale);
        Path stage = Files.createTempFile(saved.getParent(), "ubuntu-account", ".tmp");
        try {
            try (OutputStream stream = Files.newOutputStream(stage)) { account.store(stream, null); }
            Files.move(stage, saved, StandardCopyOption.ATOMIC_MOVE);
        } finally {
            Files.deleteIfExists(stage);
        }
    }

    private static List<String> runSetup(PtySession.Command command, Path temporary, String prefix, byte[] password,
            Consumer<String> progress) throws IOException, InterruptedException {
        Path log = Files.createTempFile(temporary, "ubuntu-setup", ".log");
        java.lang.Process process = null;
        try {
            ProcessBuilder builder = new ProcessBuilder(command.arguments).directory(command.directory)
                    .redirectErrorStream(true).redirectOutput(log.toFile());
            builder.environment().clear();
            for (String setting : command.environment) {
                int equals = setting.indexOf('=');
                builder.environment().put(setting.substring(0, equals), setting.substring(equals + 1));
            }
            process = builder.start();
            try (OutputStream input = process.getOutputStream()) {
                input.write(prefix.getBytes(StandardCharsets.UTF_8));
                input.write(password);
                input.write('\n');
            }
            try (RandomAccessFile progressLog = new RandomAccessFile(log.toFile(), "r")) {
                while (!process.waitFor(1, TimeUnit.SECONDS)) {
                    String line;
                    while ((line = progressLog.readLine()) != null) {
                        if (line.startsWith("GALAXY_STAGE=")) {
                            progress.accept(new String(line.substring(13).getBytes(StandardCharsets.ISO_8859_1),
                                    StandardCharsets.UTF_8));
                        }
                    }
                }
            }
            List<String> lines = Files.readAllLines(log, StandardCharsets.UTF_8);
            if (process.exitValue() != 0) {
                String detail = "종료 코드 " + process.exitValue();
                for (String line : lines) if (!line.trim().isEmpty()) detail = line.trim();
                if (detail.startsWith("GALAXY_ERROR=")) detail = detail.substring(13);
                throw new IOException("Ubuntu 초기 설정 실패: " + detail.substring(0, Math.min(250, detail.length())));
            }
            return lines;
        } finally {
            if (process != null && process.isAlive()) process.destroyForcibly();
            Files.deleteIfExists(log);
        }
    }

    private static Path install(Context context, Consumer<String> progress) throws IOException, InterruptedException {
        Path directory = context.getNoBackupFilesDir().toPath();
        Path root = directory.resolve("ubuntu-24.04");
        Path staging = directory.resolve("ubuntu.installing");
        INSTALL_LOCK.lockInterruptibly();
        try {
            if (Files.exists(root, LinkOption.NOFOLLOW_LINKS)) {
                if (!isRootfs(root)) throw new IOException("기존 Ubuntu 설치에서 Bash 또는 apt를 찾을 수 없습니다.");
                return root;
            }
            removeTree(staging);
            Files.createDirectory(staging);
            try {
                Path archive = staging.resolve("ubuntu.tar.gz");
                download(archive, progress);
                progress.accept("Ubuntu 설치 중");
                Path extracted = Files.createDirectory(staging.resolve("rootfs"));
                extract(archive, extracted, staging.resolve("tar.log"));
                if (!isRootfs(extracted)) throw new IOException("Ubuntu 설치 파일에 Bash 또는 apt가 없습니다.");
                Files.createDirectories(extracted.resolve("tmp"));
                Files.move(extracted, root, StandardCopyOption.ATOMIC_MOVE);
                return root;
            } finally {
                try { removeTree(staging); }
                catch (IOException error) { Log.w("GalaxyTerminal", "Ubuntu staging cleanup failed", error); }
            }
        } finally {
            INSTALL_LOCK.unlock();
        }
    }

    private static boolean isRootfs(Path root) {
        return Files.isDirectory(root, LinkOption.NOFOLLOW_LINKS)
                && Files.isRegularFile(root.resolve("usr/bin/bash"))
                && Files.isRegularFile(root.resolve("usr/bin/apt"));
    }

    private static void download(Path destination, Consumer<String> progress) throws IOException, InterruptedException {
        MessageDigest sha256;
        try { sha256 = MessageDigest.getInstance("SHA-256"); }
        catch (NoSuchAlgorithmException error) { throw new IOException(error); }
        HttpURLConnection request = (HttpURLConnection) new URL(IMAGE_URL).openConnection();
        request.setConnectTimeout(15_000);
        request.setReadTimeout(20_000);
        request.setInstanceFollowRedirects(false);
        request.setRequestProperty("Accept-Encoding", "identity");
        progress.accept("Ubuntu 다운로드 0%");
        try {
            int status = request.getResponseCode();
            if (status != HttpURLConnection.HTTP_OK) throw new IOException("Ubuntu 다운로드 실패 (HTTP " + status + ")");
            long received = 0;
            int lastProgress = 0;
            try (InputStream source = request.getInputStream(); OutputStream output = Files.newOutputStream(destination)) {
                byte[] buffer = new byte[64 * 1024];
                int count;
                while ((count = source.read(buffer)) != -1) {
                    if (Thread.currentThread().isInterrupted()) throw new InterruptedException();
                    received += count;
                    if (received > IMAGE_SIZE) throw new IOException("Ubuntu 이미지 크기가 예상과 다릅니다.");
                    sha256.update(buffer, 0, count);
                    output.write(buffer, 0, count);
                    int percent = (int) (received * 100 / IMAGE_SIZE);
                    if (percent >= lastProgress + 5 || percent == 100) {
                        lastProgress = percent;
                        progress.accept("Ubuntu 다운로드 " + percent + "%");
                    }
                }
            }
            StringBuilder hash = new StringBuilder(64);
            for (byte value : sha256.digest()) {
                hash.append(Character.forDigit((value & 0xff) >>> 4, 16));
                hash.append(Character.forDigit(value & 0xf, 16));
            }
            if (received != IMAGE_SIZE || !IMAGE_SHA256.contentEquals(hash)) {
                throw new IOException("Ubuntu 이미지의 SHA-256 검증에 실패했습니다.");
            }
        } finally {
            request.disconnect();
        }
    }

    private static void extract(Path archive, Path directory, Path log) throws IOException, InterruptedException {
        List<String> arguments = new ArrayList<>(Arrays.asList("/system/bin/toybox", "tar", "-xzf",
                archive.toString(), "-C", directory.toString()));
        for (String[] link : IMAGE_HARD_LINKS) arguments.add("--exclude=" + link[0]);
        java.lang.Process tar = new ProcessBuilder(arguments).redirectErrorStream(true).redirectOutput(log.toFile()).start();
        int status;
        try {
            status = tar.waitFor();
        } catch (InterruptedException stopping) {
            tar.destroyForcibly();
            tar.waitFor();
            throw stopping;
        }
        if (status != 0) {
            try (BufferedReader error = Files.newBufferedReader(log, StandardCharsets.UTF_8)) {
                String detail = error.readLine();
                throw new IOException("Ubuntu 압축을 풀 수 없습니다 (tar " + status + ")"
                        + (detail == null ? "" : ": " + detail));
            }
        }
        // Android's app sandbox denies hard links. Relative symlinks survive the install directory move.
        for (String[] link : IMAGE_HARD_LINKS) {
            Files.createSymbolicLink(directory.resolve(link[0]), Paths.get(link[1]));
        }
    }

    private static void copyAsset(Context context, String name, Path destination) throws IOException {
        try (InputStream source = context.getAssets().open(name)) {
            Files.copy(source, destination, StandardCopyOption.REPLACE_EXISTING);
        }
    }

    private static String dnsConfiguration(Context context) {
        ConnectivityManager connectivity = context.getSystemService(ConnectivityManager.class);
        Network network = connectivity == null ? null : connectivity.getActiveNetwork();
        LinkProperties link = network == null ? null : connectivity.getLinkProperties(network);
        StringBuilder resolv = new StringBuilder();
        if (link != null) {
            int count = 0;
            for (InetAddress address : link.getDnsServers()) {
                if (!address.isAnyLocalAddress() && !address.isLoopbackAddress() && !address.isLinkLocalAddress()) {
                    resolv.append("nameserver ").append(address.getHostAddress()).append('\n');
                    if (++count == 3) break;
                }
            }
        }
        return resolv.length() == 0 ? "nameserver 1.1.1.1\nnameserver 8.8.8.8\n" : resolv.toString();
    }

    private static void removeTree(Path directory) throws IOException {
        if (!Files.exists(directory, LinkOption.NOFOLLOW_LINKS)) return;
        Files.walkFileTree(directory, new SimpleFileVisitor<Path>() {
            @Override public FileVisitResult visitFile(Path file, BasicFileAttributes attributes) throws IOException {
                Files.delete(file);
                return FileVisitResult.CONTINUE;
            }

            @Override public FileVisitResult postVisitDirectory(Path folder, IOException error) throws IOException {
                if (error != null) throw error;
                Files.delete(folder);
                return FileVisitResult.CONTINUE;
            }
        });
    }
}
