#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

static void require(int condition, const char *message) {
    if (!condition) {
        fprintf(stderr, "%s: %s\n", message, strerror(errno));
        exit(1);
    }
}

static struct sockaddr_storage loopback(int family, socklen_t *length) {
    struct sockaddr_storage address = {0};
    if (family == AF_INET) {
        struct sockaddr_in *v4 = (struct sockaddr_in *)&address;
        v4->sin_family = AF_INET;
        v4->sin_addr.s_addr = htonl(INADDR_LOOPBACK);
        *length = sizeof(*v4);
    } else {
        struct sockaddr_in6 *v6 = (struct sockaddr_in6 *)&address;
        v6->sin6_family = AF_INET6;
        v6->sin6_addr = in6addr_loopback;
        *length = sizeof(*v6);
    }
    return address;
}

static int option(int fd, int name) {
    int value = -1;
    socklen_t length = sizeof(value);
    require(getsockopt(fd, SOL_SOCKET, name, &value, &length) == 0, "read socket option");
    return value;
}

static void restart(int family, int enabled) {
    socklen_t length;
    struct sockaddr_storage address = loopback(family, &length);
    int listener = socket(family, SOCK_STREAM, 0);
    if (family == AF_INET6 && listener < 0 && errno == EAFNOSUPPORT) {
        puts("IPv6 is unavailable; skipping IPv6 socket checks");
        return;
    }
    require(listener >= 0, "create TCP listener");
    int result = bind(listener, (struct sockaddr *)&address, length);
    if (family == AF_INET6 && result < 0 && errno == EADDRNOTAVAIL) {
        close(listener);
        puts("IPv6 loopback is unavailable; skipping IPv6 socket checks");
        return;
    }
    require(result == 0, "bind TCP listener");
    require(option(listener, SO_REUSEADDR) == enabled, "TCP reuse setting");
    require(option(listener, SO_REUSEPORT) == 0, "SO_REUSEPORT must stay disabled");
    require(getsockname(listener, (struct sockaddr *)&address, &length) == 0, "find listener port");
    require(listen(listener, 1) == 0, "listen");

    // Address reuse must never allow a second active listener on the same port.
    int competitor = socket(family, SOCK_STREAM, 0);
    require(competitor >= 0, "create competing socket");
    require(bind(competitor, (struct sockaddr *)&address, length) == -1 && errno == EADDRINUSE,
            "reject competing listener");
    close(competitor);

    int client = socket(family, SOCK_STREAM, 0);
    require(client >= 0, "create client");
    require(connect(client, (struct sockaddr *)&address, length) == 0, "connect client");
    int accepted = accept(listener, NULL, NULL);
    require(accepted >= 0, "accept client");
    // The server actively closes, just like NetherNet's Connection: close
    // response. The server port enters TIME_WAIT after the client closes.
    close(accepted);
    char byte;
    require(recv(client, &byte, 1, 0) == 0, "receive server FIN");
    close(client);
    close(listener);
    struct timespec delay = {.tv_nsec = 20000000};
    while (nanosleep(&delay, &delay) == -1 && errno == EINTR) {}

    int replacement = socket(family, SOCK_STREAM, 0);
    require(replacement >= 0, "create replacement listener");
    result = bind(replacement, (struct sockaddr *)&address, length);
    if (enabled) {
        require(result == 0, "rebind TCP port in TIME_WAIT");
        require(listen(replacement, 1) == 0, "listen after restart");
    } else {
        require(result == -1 && errno == EADDRINUSE, "baseline must reproduce blocked restart");
    }
    close(replacement);
}

int main(int argc, char **argv) {
    require(argc == 2, "expected baseline or enabled argument");
    int enabled = strcmp(argv[1], "enabled") == 0;
    restart(AF_INET, enabled);
    restart(AF_INET6, enabled);

    socklen_t length;
    struct sockaddr_storage address = loopback(AF_INET, &length);
    int udp = socket(AF_INET, SOCK_DGRAM, 0);
    require(udp >= 0, "create UDP socket");
    int before = option(udp, SO_REUSEADDR);
    require(bind(udp, (struct sockaddr *)&address, length) == 0, "bind UDP socket");
    require(option(udp, SO_REUSEADDR) == before, "leave UDP socket unchanged");
    close(udp);

    require(bind(-1, NULL, 0) == -1 && errno == EBADF, "preserve invalid-fd error");
    printf("%s: restart, active-listener exclusion, UDP, and error checks passed\n", argv[1]);
    return 0;
}
