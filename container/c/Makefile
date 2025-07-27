# Makefile for Catnip Title Interceptor

CC = gcc
CFLAGS = -Wall -Wextra -fPIC -shared -O2 -g
LDFLAGS = -ldl -lpthread

# Target library
TARGET = libtitle_interceptor.so

# Source file
SRC = title_interceptor.c

# Default target
all: $(TARGET)

# Build the shared library
$(TARGET): $(SRC)
	$(CC) $(CFLAGS) -o $(TARGET) $(SRC) $(LDFLAGS)

# Install to system location (requires sudo)
install: $(TARGET)
	sudo cp $(TARGET) /usr/local/lib/
	sudo ldconfig

# Clean build artifacts
clean:
	rm -f $(TARGET)

.PHONY: all install clean