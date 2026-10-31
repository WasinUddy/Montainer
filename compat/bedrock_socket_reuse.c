// Mojang's NetherNet HTTP listener does not enable SO_REUSEADDR. Its closed
// connections can keep server-port in TIME_WAIT and prevent the next Bedrock
// process from listening after a graceful restart. Apply TCP address reuse
// before bind; do not enable SO_REUSEPORT or change UDP socket behavior.
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <netinet/in.h>
#include <pthread.h>
#include <sys/socket.h>

typedef int (*bind_function)(int, const struct sockaddr *, socklen_t);
static bind_function original_bind;
static pthread_once_t resolve_once = PTHREAD_ONCE_INIT;

static void resolve_bind(void) {
    original_bind = (bind_function)dlsym(RTLD_NEXT, "bind");
}

int bind(int fd, const struct sockaddr *address, socklen_t length) {
    int saved_errno = errno;
    if (pthread_once(&resolve_once, resolve_bind) != 0 || original_bind == NULL) {
        errno = ENOSYS;
        return -1;
    }

    if (address != NULL && length >= sizeof(address->sa_family) &&
        (address->sa_family == AF_INET || address->sa_family == AF_INET6)) {
        int type = 0, protocol = 0;
        socklen_t option_length = sizeof(type);
        if (getsockopt(fd, SOL_SOCKET, SO_TYPE, &type, &option_length) == 0 &&
            type == SOCK_STREAM) {
            option_length = sizeof(protocol);
            if (getsockopt(fd, SOL_SOCKET, SO_PROTOCOL, &protocol, &option_length) == 0 &&
                protocol == IPPROTO_TCP) {
                int enabled = 1;
                // If the runtime disallows this option, preserve bind's normal
                // result rather than inventing a successful bind or new error.
                (void)setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &enabled, sizeof(enabled));
            }
        }
    }

    errno = saved_errno;
    return original_bind(fd, address, length);
}
