// Ubuntu ARM64 and Android LP64 have the same on-disk utmp layout.
// Bionic's utmp functions are stubs, so lock the database as glibc does.
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <signal.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/stat.h>
#include <unistd.h>
#include <utmp.h>

_Static_assert(sizeof(struct utmp) == 400 && offsetof(struct utmp, ut_tv) == 344, "Ubuntu arm64 utmp ABI");

enum change { LOGIN, LOGOUT, CLOSE_WINDOW };

static int tty_id(const char *line, char id[4]) {
    unsigned index;
    char extra;
    if (sscanf(line, "pts/%u%c", &index, &extra) != 1 || index > 0xffffff) return 0;
    id[0] = 'G';
    id[1] = index >> 16;
    id[2] = index >> 8;
    id[3] = index;
    return 1;
}

static int account(const struct utmp *session, enum change change) {
    // The Android close callback runs in the rootfs directory after PRoot stops.
    int dir = open(change == CLOSE_WINDOW ? "run" : "/run", O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC);
    if (dir < 0) return -1;
    int flags = O_RDWR | O_NOFOLLOW | O_CLOEXEC | O_NONBLOCK;
    int fd = openat(dir, "utmp", flags | (change == LOGIN ? O_CREAT : 0), 0644);
    close(dir);
    if (fd < 0 && change == CLOSE_WINDOW && errno == ENOENT) return 0;
    if (fd < 0) return -1;
    int result = -1;
    struct flock lock = {.l_type = F_WRLCK, .l_whence = SEEK_SET};
    if (fcntl(fd, F_SETLKW, &lock) < 0) goto done;
    struct stat info;
    if (fstat(fd, &info) < 0) goto done;
    if (!S_ISREG(info.st_mode) || info.st_size % sizeof(struct utmp)) {
        errno = EINVAL;
        goto done;
    }
    off_t slot = info.st_size;
    for (off_t pos = 0; pos < info.st_size; pos += sizeof(struct utmp)) {
        struct utmp row;
        if (pread(fd, &row, sizeof(row), pos) != sizeof(row)) goto done;
        char id[4];
        if (!memchr(row.ut_line, 0, sizeof(row.ut_line)) || !tty_id(row.ut_line, id) ||
            memcmp(id, row.ut_id, sizeof(id))) continue;
        if (row.ut_type == USER_PROCESS) {
            int same = !memcmp(row.ut_line, session->ut_line, sizeof(row.ut_line));
            int ours = same && row.ut_pid == session->ut_pid &&
                row.ut_tv.tv_sec == session->ut_tv.tv_sec && row.ut_tv.tv_usec == session->ut_tv.tv_usec;
            int stale = row.ut_pid <= 0 || (kill(row.ut_pid, 0) < 0 && errno == ESRCH);
            if (change == LOGIN && same && !stale) { result = 0; goto done; }
            if (!stale && !(change == LOGOUT && ours) &&
                !(change == CLOSE_WINDOW && row.ut_session == session->ut_session)) continue;
            row.ut_type = DEAD_PROCESS;
            memset(row.ut_user, 0, sizeof(row.ut_user));
            gettimeofday(&row.ut_tv, NULL);
            if (pwrite(fd, &row, sizeof(row), pos) != sizeof(row)) goto done;
        }
        if (row.ut_type == DEAD_PROCESS && slot == info.st_size) slot = pos;
    }
    if (change == LOGIN && pwrite(fd, session, sizeof(*session), slot) != sizeof(*session)) goto done;
    result = change == LOGIN;
done: {
        int saved = errno;
        close(fd);
        errno = saved;
        return result;
    }
}

int main(int argc, char **argv) {
    struct utmp session = {.ut_type = USER_PROCESS, .ut_pid = getppid()};
    int closing = argc == 3 && !strcmp(argv[1], "--close-window");
    const char *window = closing ? argv[2] : getenv("GALAXY_UTMP_WINDOW");
    char *end;
    if (!window) return 1;
    session.ut_session = strtol(window, &end, 10);
    if (*end || session.ut_session <= 0) return 1;
    if (closing) return account(&session, CLOSE_WINDOW) < 0;
    const char *tty = ttyname(STDIN_FILENO);
    if (argc != 3 || session.ut_pid != atoi(argv[1]) || !tty ||
        strncmp(tty, "/dev/", 5) || !tty_id(tty + 5, session.ut_id) ||
        !*argv[2] || strlen(argv[2]) > sizeof(session.ut_user)) return 1;
    snprintf(session.ut_line, sizeof(session.ut_line), "%s", tty + 5);
    memcpy(session.ut_user, argv[2], strlen(argv[2]));
    if (gettimeofday(&session.ut_tv, NULL) < 0) return 1;
    sigset_t signals;
    sigemptyset(&signals);
    sigaddset(&signals, SIGUSR1);
    sigaddset(&signals, SIGHUP);
    if (sigprocmask(SIG_BLOCK, &signals, NULL) || prctl(PR_SET_PDEATHSIG, SIGUSR1) ||
        getppid() != session.ut_pid) return 1;
    int added = account(&session, LOGIN);
    if (added < 0) { perror("Galaxy Terminal: utmp"); return 1; }
    if (write(STDOUT_FILENO, "\n", 1) != 1 || !added) {
        if (added) account(&session, LOGOUT);
        return 0;
    }
    close(STDIN_FILENO);
    close(STDOUT_FILENO);
    close(STDERR_FILENO);
    int signal;
    while (sigwait(&signals, &signal) == EINTR) {}
    return account(&session, LOGOUT) < 0;
}
