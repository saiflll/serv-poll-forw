
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Salin informasi modul dan folder vendor
COPY go.mod go.sum ./
COPY vendor ./vendor

# Verifikasi bahwa dependensi di folder vendor sudah sesuai
RUN go mod verify

# Salin sisa kode aplikasi
COPY . .

# Build aplikasi Go menggunakan dependensi dari vendor
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -installsuffix cgo -o /servfi ./main.go


# --- Tahap Final ---
FROM alpine:latest

WORKDIR /app

# Salin file binary yang sudah di-build dari tahap sebelumnya
COPY --from=builder /servfi .

# Perintah untuk menjalankan aplikasi
CMD ["./servfi"]
