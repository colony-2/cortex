/*
 * intercept.c  –  LD_PRELOAD shim for read/write
 *
 * Build inside the Dockerfile (see below).
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <string.h>
#include <stdio.h>
#include <errno.h>
#include <pthread.h>
#include <limits.h>

static const char *SOCK_PATH = "/tmp/monitor.sock";
static ssize_t (*real_write)(int,const void*,size_t) = NULL;
static ssize_t (*real_read) (int,void*,size_t)       = NULL;

static void init_syms(void) {
    real_write = dlsym(RTLD_NEXT, "write");
    real_read  = dlsym(RTLD_NEXT, "read");
    if (!real_write || !real_read) {
        const char *e = dlerror(); write(2,e,strlen(e)); _exit(1);
    }
}

static int ask_ok(const char *op,int fd,size_t cnt) {
    int s = socket(AF_UNIX, SOCK_STREAM, 0);
    if (s==-1) return 1;
    struct sockaddr_un addr={0}; addr.sun_family=AF_UNIX;
    strncpy(addr.sun_path, SOCK_PATH, sizeof(addr.sun_path)-1);
    if (connect(s,(struct sockaddr*)&addr,sizeof(addr))==-1){close(s);return 1;}
    
    // Get filename from file descriptor
    char path[PATH_MAX];
    char linkpath[64];
    snprintf(linkpath, sizeof(linkpath), "/proc/self/fd/%d", fd);
    ssize_t len = readlink(linkpath, path, sizeof(path)-1);
    if (len > 0) {
        path[len] = '\0';
    } else {
        // Handle special file descriptors
        if (fd == 0) strcpy(path, "stdin");
        else if (fd == 1) strcpy(path, "stdout");
        else if (fd == 2) strcpy(path, "stderr");
        else snprintf(path, sizeof(path), "fd:%d", fd);
    }
    
    char msg[512]; 
    int n=snprintf(msg,sizeof(msg),"%s %d %zu %s\n",op,fd,cnt,path);
    if (send(s,msg,n,0)!=n){close(s);return 1;}
    char rep[8]={0}; int r=recv(s,rep,sizeof(rep)-1,0); close(s);
    if (r<=0) return 1;
    return strncmp(rep,"ALLOW",5)==0;
}

ssize_t write(int fd,const void*buf,size_t c){
    static pthread_once_t o=PTHREAD_ONCE_INIT; pthread_once(&o,init_syms);
    if(!ask_ok("WRITE",fd,c)){errno=EPERM;return-1;}
    return real_write(fd,buf,c);
}

ssize_t read(int fd,void*buf,size_t c){
    static pthread_once_t o=PTHREAD_ONCE_INIT; pthread_once(&o,init_syms);
    if(!ask_ok("READ",fd,c)){errno=EPERM;return-1;}
    return real_read(fd,buf,c);
}
