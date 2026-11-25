package dev.galaxy.terminal;

import android.os.ParcelFileDescriptor;
import android.system.ErrnoException;
import android.system.Os;
import android.system.StructPollfd;
import java.io.File;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.ArrayDeque;

import static android.system.OsConstants.*;

final class PtySession implements AutoCloseable {
    interface Listener {
        void started();
        void output(byte[] bytes, int length, boolean echoErases) throws InterruptedException;
        void ended(String message);
    }

    interface Launch {
        Command prepare() throws IOException, InterruptedException;
    }

    interface Cleanup { void run() throws IOException; }

    static final class Command {
        final File directory;
        final String[] arguments;
        final String[] environment;
        Cleanup onExit;

        Command(File directory, String[] arguments, String... environment) {
            this.directory = directory;
            this.arguments = arguments;
            this.environment = environment;
        }
    }

    private static final int INPUT_LIMIT = 256 * 1024;
    private static final byte[] WAKE = { 1 };

    static { System.loadLibrary("galaxy_pty"); }

    private static native int[] spawn(String directory, String[] arguments, String[] environment, int cols, int rows)
            throws IOException;
    private static native void resize(int fd, int cols, int rows) throws IOException;
    private static native boolean echoErases(int fd);
    private static native void reap(int fd, int pid);

    private final ParcelFileDescriptor[] wake;
    private final ArrayDeque<ByteBuffer> input = new ArrayDeque<>();
    private final Thread worker;
    private volatile boolean closed;
    private volatile int size;
    private int pendingBytes;

    PtySession(Launch launch, int cols, int rows, Listener listener)
            throws IOException, ErrnoException {
        size = cols << 16 | rows;
        wake = ParcelFileDescriptor.createPipe();
        try {
            Os.fcntlInt(wake[1].getFileDescriptor(), F_SETFL, O_NONBLOCK);
        } catch (ErrnoException error) {
            closeWake();
            throw error;
        }
        worker = new Thread(() -> run(launch, listener), "terminal-pty");
        worker.start();
    }

    synchronized boolean write(byte[] bytes) {
        if (closed || bytes.length > INPUT_LIMIT - pendingBytes) return false;
        if (bytes.length > 0) {
            input.add(ByteBuffer.wrap(bytes));
            pendingBytes += bytes.length;
            signal();
        }
        return true;
    }

    synchronized void resize(int cols, int rows) {
        int next = cols << 16 | rows;
        if (!closed && size != next) {
            size = next;
            signal();
        }
    }

    @Override public synchronized void close() {
        if (!closed) {
            closed = true;
            signal();
            worker.interrupt();
        }
    }

    private void signal() {
        try {
            Os.write(wake[1].getFileDescriptor(), WAKE, 0, WAKE.length);
        } catch (ErrnoException error) {
            if (error.errno != EAGAIN && error.errno != EINTR) worker.interrupt();
        } catch (IOException error) {
            worker.interrupt();
        }
    }

    private void run(Launch launch, Listener listener) {
        String message = "셸이 종료되었습니다.";
        try {
            Command command = launch.prepare();
            if (closed || Thread.currentThread().isInterrupted()) return;
            int initial = size;
            int[] child = spawn(command.directory.getPath(), command.arguments, command.environment,
                    initial >>> 16, initial & 0xffff);
            try (ParcelFileDescriptor master = ParcelFileDescriptor.adoptFd(child[0])) {
                try {
                    listener.started();
                    pump(master, initial, listener);
                } finally {
                    reap(master.getFd(), child[1]);
                    if (command.onExit != null) command.onExit.run();
                }
            }
        } catch (IOException | ErrnoException error) {
            message = "셸 오류: " + error.getMessage();
        } catch (InterruptedException stopping) {
            Thread.currentThread().interrupt();
        } finally {
            synchronized (this) {
                closed = true;
                closeWake();
            }
            listener.ended(message);
        }
    }

    private void pump(ParcelFileDescriptor master, int appliedSize, Listener listener)
            throws ErrnoException, IOException, InterruptedException {
        StructPollfd pty = new StructPollfd();
        pty.fd = master.getFileDescriptor();
        StructPollfd notification = new StructPollfd();
        notification.fd = wake[0].getFileDescriptor();
        notification.events = (short) POLLIN;
        StructPollfd[] descriptors = { pty, notification };
        byte[] buffer = new byte[16 * 1024];

        while (!closed && !Thread.currentThread().isInterrupted()) {
            int nextSize = size;
            if (appliedSize != nextSize) {
                resize(master.getFd(), nextSize >>> 16, nextSize & 0xffff);
                appliedSize = nextSize;
            }
            ByteBuffer command;
            synchronized (this) { command = input.peek(); }
            pty.events = (short) (POLLIN | (command == null ? 0 : POLLOUT));
            try {
                Os.poll(descriptors, -1);
                if (closed) return;
                if ((notification.revents & POLLIN) != 0) Os.read(notification.fd, buffer, 0, buffer.length);
                if ((pty.revents & POLLIN) != 0) {
                    int length = Os.read(pty.fd, buffer, 0, buffer.length);
                    if (length <= 0) return;
                    listener.output(buffer, length, echoErases(master.getFd()));
                } else if ((pty.revents & (POLLHUP | POLLERR | POLLNVAL)) != 0) {
                    return;
                }
                if (command != null && (pty.revents & POLLOUT) != 0) {
                    int count = Os.write(pty.fd, command.array(), command.position(), command.remaining());
                    command.position(command.position() + count);
                    synchronized (this) {
                        pendingBytes -= count;
                        if (!command.hasRemaining()) input.remove();
                    }
                }
            } catch (ErrnoException error) {
                if (error.errno == EIO) return;
                if (error.errno != EAGAIN && error.errno != EINTR) throw error;
            }
        }
    }

    private void closeWake() {
        for (ParcelFileDescriptor descriptor : wake) {
            try { descriptor.close(); } catch (IOException ignored) { }
        }
    }
}
