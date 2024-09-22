# Use Golang image based on Debian Bookworm
FROM golang:bookworm

# Set the working directory within the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application
RUN go build -o main .

# Expose the port that the application will run on
EXPOSE 3334

# Set the command to run the executable
CMD ["./main"]