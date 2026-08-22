# ビルド環境を実行イメージに残さないためのマルチステージ。
FROM golang:1.26-alpine AS build

WORKDIR /src

# 依存だけ先に解決する。ソースだけ変えたときにこの層が再利用される。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO を切って静的リンクにする。ベースイメージの libc に依存しなくなるので、
# distroless の static にそのまま載る。
#
# -trimpath はビルドマシンの絶対パスをバイナリから消す。
# -w -s はデバッグ情報を落としてサイズを減らす。
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-w -s" -o /out/api ./cmd/api

# distroless にはシェルもパッケージマネージャも入っていない。
# 侵入されても足場が無い。
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api

# Cloud Run は PORT を環境変数で渡してくる。ここは既定値。
ENV PORT=8080
EXPOSE 8080

# 非 root で動かす。distroless の nonroot タグが uid 65532 を用意している。
USER nonroot:nonroot

ENTRYPOINT ["/api"]
