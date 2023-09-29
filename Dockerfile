# Use the official Golang image as the base image
FROM golang:1.19 as builder

LABEL maintainer="Hrishikesh Sawant <your@email.com>"
LABEL org.opencontainers.image.title="atlas-backend"

# Set the working directory inside the container
WORKDIR /app

# Copy your Go source code into the container
COPY . .

# Build your Go application
RUN go build -o atlas-backend

# Create a smaller final image
FROM debian:bullseye-slim

# Copy the built binary from the builder stage into the final image
COPY --from=builder /app/atlas-backend /app/atlas-backend

# Expose the port that your application listens on (8080 in this case)
EXPOSE 8080

# Run your application when the container starts
CMD ["/app/atlas-backend"]
