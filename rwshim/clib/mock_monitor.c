/*
 * mock_monitor.c - Mock monitor service for testing the intercept shim
 * Creates a domain socket server that responds to read/write requests
 */
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <string.h>
#include <signal.h>
#include <errno.h>

static const char *SOCK_PATH = "/tmp/monitor.sock";
static int server_fd = -1;

typedef enum {
    POLICY_ALLOW_ALL,
    POLICY_DENY_ALL,
    POLICY_DENY_WRITES,
    POLICY_DENY_READS
} policy_t;

static policy_t current_policy = POLICY_ALLOW_ALL;

void cleanup(int sig) {
    if (server_fd != -1) {
        close(server_fd);
    }
    unlink(SOCK_PATH);
    exit(0);
}

void handle_request(int client_fd) {
    char buffer[512];
    ssize_t bytes = recv(client_fd, buffer, sizeof(buffer) - 1, 0);
    if (bytes <= 0) {
        close(client_fd);
        return;
    }
    
    buffer[bytes] = '\0';
    
    char op[16], filename[256];
    int fd;
    size_t count;
    
    // Parse the message including filename
    int parsed = sscanf(buffer, "%15s %d %zu %255s", op, &fd, &count, filename);
    if (parsed < 3) {
        send(client_fd, "DENY\n", 5, 0);
        close(client_fd);
        return;
    }
    
    if (parsed == 3) {
        strcpy(filename, "unknown");
    }
    
    printf("Monitor received: %s fd=%d size=%zu file=%s\n", op, fd, count, filename);
    
    const char *reply = "DENY\n";
    
    switch (current_policy) {
        case POLICY_ALLOW_ALL:
            reply = "ALLOW\n";
            break;
        case POLICY_DENY_ALL:
            reply = "DENY\n";
            break;
        case POLICY_DENY_WRITES:
            reply = (strcmp(op, "WRITE") == 0) ? "DENY\n" : "ALLOW\n";
            break;
        case POLICY_DENY_READS:
            reply = (strcmp(op, "READ") == 0) ? "DENY\n" : "ALLOW\n";
            break;
    }
    
    printf("Monitor responding: %s", reply);
    send(client_fd, reply, strlen(reply), 0);
    close(client_fd);
}

int main(int argc, char *argv[]) {
    if (argc > 1) {
        if (strcmp(argv[1], "deny-all") == 0) {
            current_policy = POLICY_DENY_ALL;
        } else if (strcmp(argv[1], "deny-writes") == 0) {
            current_policy = POLICY_DENY_WRITES;
        } else if (strcmp(argv[1], "deny-reads") == 0) {
            current_policy = POLICY_DENY_READS;
        }
    }
    
    printf("Mock monitor starting with policy: %d\n", current_policy);
    
    signal(SIGINT, cleanup);
    signal(SIGTERM, cleanup);
    
    unlink(SOCK_PATH);
    
    server_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (server_fd == -1) {
        perror("socket");
        return 1;
    }
    
    struct sockaddr_un addr = {0};
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, SOCK_PATH, sizeof(addr.sun_path) - 1);
    
    if (bind(server_fd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
        perror("bind");
        close(server_fd);
        return 1;
    }
    
    if (listen(server_fd, 5) == -1) {
        perror("listen");
        close(server_fd);
        unlink(SOCK_PATH);
        return 1;
    }
    
    printf("Mock monitor listening on %s\n", SOCK_PATH);
    
    while (1) {
        int client_fd = accept(server_fd, NULL, NULL);
        if (client_fd == -1) {
            if (errno == EINTR) continue;
            perror("accept");
            break;
        }
        
        handle_request(client_fd);
    }
    
    cleanup(0);
    return 0;
}