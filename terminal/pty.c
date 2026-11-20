#include <errno.h>
#include <fcntl.h>
#include <jni.h>
#include <pty.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

static void io_error(JNIEnv *env, const char *operation) {
    char message[192];
    snprintf(message, sizeof(message), "%s: %s", operation, strerror(errno));
    jclass error = (*env)->FindClass(env, "java/io/IOException");
    if (error) (*env)->ThrowNew(env, error, message);
}

static void free_strings(char **strings) {
    if (!strings) return;
    for (char **value = strings; *value; ++value) free(*value);
    free(strings);
}

static char **copy_strings(JNIEnv *env, jobjectArray array) {
    jsize count = (*env)->GetArrayLength(env, array);
    char **strings = calloc((size_t)count + 1, sizeof(*strings));
    if (!strings) {
        io_error(env, "calloc");
        return NULL;
    }
    for (jsize i = 0; i < count; ++i) {
        jstring item = (*env)->GetObjectArrayElement(env, array, i);
        const char *text = item ? (*env)->GetStringUTFChars(env, item, NULL) : NULL;
        if (text) {
            strings[i] = strdup(text);
            (*env)->ReleaseStringUTFChars(env, item, text);
        }
        (*env)->DeleteLocalRef(env, item);
        if (!strings[i]) {
            if (!(*env)->ExceptionCheck(env)) io_error(env, "PTY arguments");
            free_strings(strings);
            return NULL;
        }
    }
    return strings;
}

static void finish(int master, pid_t child) {
    pid_t foreground = tcgetpgrp(master);
    pid_t result;
    do result = waitpid(child, NULL, WNOHANG); while (result < 0 && errno == EINTR);
    if (result != 0) return;

    if (foreground > 0 && foreground != child) kill(-foreground, SIGHUP);
    kill(-child, SIGKILL);
    kill(child, SIGKILL);
    do result = waitpid(child, NULL, 0); while (result < 0 && errno == EINTR);
}

JNIEXPORT jintArray JNICALL Java_dev_galaxy_terminal_PtySession_spawn(
        JNIEnv *env, jclass type __attribute__((unused)), jstring directory,
        jobjectArray arguments, jobjectArray environment, jint cols, jint rows) {
    jintArray result = (*env)->NewIntArray(env, 2);
    if (!result) return NULL;
    char **args = copy_strings(env, arguments);
    if (!args) return NULL;
    char **vars = copy_strings(env, environment);
    const char *working_dir = vars ? (*env)->GetStringUTFChars(env, directory, NULL) : NULL;
    if (!working_dir) {
        free_strings(args);
        free_strings(vars);
        return NULL;
    }
    struct winsize size = { .ws_row = rows, .ws_col = cols };
    int master;
    pid_t child = forkpty(&master, NULL, NULL, &size);
    if (child == 0) {
        struct termios tty;
        if (tcgetattr(STDIN_FILENO, &tty) == 0) {
            tty.c_iflag |= IUTF8;
            tty.c_cc[VERASE] = 127;
            tcsetattr(STDIN_FILENO, TCSANOW, &tty);
        }
        sigset_t mask;
        sigemptyset(&mask);
        sigprocmask(SIG_SETMASK, &mask, NULL);
        if (chdir(working_dir) == 0) execve(args[0], args, vars);
        const char failed[] = "Unable to start the terminal command.\r\n";
        write(STDERR_FILENO, failed, sizeof(failed) - 1);
        _exit(126);
    }
    int spawn_error = errno;
    (*env)->ReleaseStringUTFChars(env, directory, working_dir);
    free_strings(args);
    free_strings(vars);
    if (child < 0) {
        errno = spawn_error;
        io_error(env, "forkpty");
        return NULL;
    }
    if (fcntl(master, F_SETFD, FD_CLOEXEC) < 0 || fcntl(master, F_SETFL, O_NONBLOCK) < 0) {
        io_error(env, "fcntl");
        finish(master, child);
        close(master);
        return NULL;
    }
    const jint values[] = { master, child };
    (*env)->SetIntArrayRegion(env, result, 0, 2, values);
    return result;
}

JNIEXPORT void JNICALL Java_dev_galaxy_terminal_PtySession_resize(
        JNIEnv *env, jclass type __attribute__((unused)), jint master, jint cols, jint rows) {
    struct winsize size = { .ws_row = rows, .ws_col = cols };
    if (ioctl(master, TIOCSWINSZ, &size) < 0) io_error(env, "TIOCSWINSZ");
}

JNIEXPORT void JNICALL Java_dev_galaxy_terminal_PtySession_reap(
        JNIEnv *env __attribute__((unused)), jclass type __attribute__((unused)), jint master, jint child) {
    finish(master, child);
}
