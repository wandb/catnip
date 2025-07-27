# Makefile for Rust Title Interceptor

# Target library
TARGET = libtitle_interceptor.so

# Default target
all: $(TARGET)

# Build the shared library
$(TARGET):
	cargo build --release --target x86_64-unknown-linux-gnu
	cp target/x86_64-unknown-linux-gnu/release/libtitle_interceptor.so ./

# Build in Docker container
docker-build:
	docker exec -it catnip-test bash --login -c 'cd /live/catnip/container/rust-title-interceptor && cargo build --release'
	docker exec -it catnip-test bash --login -c 'cp /live/catnip/container/rust-title-interceptor/target/release/libtitle_interceptor.so /live/catnip/container/rust-title-interceptor/'

# Clean build artifacts
clean:
	cargo clean
	rm -f $(TARGET)

# Test the library with Node.js
test: docker-build
	docker exec catnip-test bash -c 'source /etc/profile.d/catnip.sh && cd /live/catnip/container/rust-title-interceptor && npm install && npm test'

.PHONY: all docker-build clean test