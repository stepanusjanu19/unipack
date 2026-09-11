module github.com/stepanusjanu19/unipack

go 1.21

// Tidak ada external dependency.
// nFPM dipanggil sebagai subprocess (bukan library) untuk hindari
// dependency tree membengkak dan memudahkan distribusi binary tunggal.
//
// Untuk build:
//   go build -o unipack ./cmd/unipack
//
// Untuk cross-compile ke Linux dari Windows/macOS:
//   GOOS=linux GOARCH=amd64 go build -o unipack ./cmd/unipack
