/*
 * test_app.c - Simple test application that performs read/write operations
 * Used to test the intercept shim functionality
 */
#include <stdio.h>
#include <unistd.h>
#include <fcntl.h>
#include <string.h>
#include <errno.h>

int main() {
    printf("Test application starting...\n");
    
    // Test 1: Write to stdout
    const char *msg = "Hello from test app!\n";
    ssize_t written = write(STDOUT_FILENO, msg, strlen(msg));
    if (written == -1) {
        printf("Write to stdout failed: %s\n", strerror(errno));
        return 1;
    }
    
    // Test 2: Create and write to a file
    int fd = open("test_output.txt", O_CREAT | O_WRONLY | O_TRUNC, 0644);
    if (fd == -1) {
        printf("Failed to open test file: %s\n", strerror(errno));
        return 1;
    }
    
    const char *file_content = "Test file content\n";
    written = write(fd, file_content, strlen(file_content));
    if (written == -1) {
        printf("Write to file failed: %s\n", strerror(errno));
        close(fd);
        return 1;
    }
    close(fd);
    
    // Test 3: Read from the file we just created
    fd = open("test_output.txt", O_RDONLY);
    if (fd == -1) {
        printf("Failed to open test file for reading: %s\n", strerror(errno));
        return 1;
    }
    
    char buffer[100];
    ssize_t bytes_read = read(fd, buffer, sizeof(buffer) - 1);
    if (bytes_read == -1) {
        printf("Read from file failed: %s\n", strerror(errno));
        close(fd);
        return 1;
    }
    buffer[bytes_read] = '\0';
    printf("Read from file: %s", buffer);
    close(fd);
    
    printf("Test application completed successfully!\n");
    return 0;
}