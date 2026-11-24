# Triển khai backend Go + web quản lý lên máy chủ (chạy trên Windows, từ thư mục gốc repo VuaThau):
#   powershell -ExecutionPolicy Bypass -File deploy\server\deploy.ps1
# Cần: Go, Node/npm, ssh/scp với host "vps" trong ~/.ssh/config. Máy chủ không phải biên dịch gì.
# App CTV bản web cho iPhone (/app/): build từ repo app Flutter nằm cạnh repo này.
# -SkipApp: không build lại, GIỮ bản /app/ đang chạy trên máy chủ (chép từ bản trước sang).
param([string]$HostName = "vps", [string]$Dir = "/opt/vuathau-nt", [switch]$SkipApp)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$stage = Join-Path $env:TEMP "vtnt-deploy"
Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force "$stage\bin", "$stage\migrations" | Out-Null

Write-Host "== web"
Push-Location "$root\frontend"; npm run build; if ($LASTEXITCODE) { throw "npm build lỗi" }; Pop-Location

$app = Join-Path (Split-Path -Parent $root) "app-Vua-Thau-"
$withApp = (-not $SkipApp) -and (Test-Path "$app\pubspec.yaml")
if ($withApp) {
  Write-Host "== app CTV bản web (/app/)"
  Push-Location $app; flutter build web --release --base-href /app/; if ($LASTEXITCODE) { throw "flutter build web lỗi" }; Pop-Location
}

Write-Host "== api (linux/amd64)"
Push-Location "$root\backend"
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o "$stage\bin\api" ./cmd/api; if ($LASTEXITCODE) { throw "go build api lỗi" }
go build -trimpath -ldflags "-s -w" -o "$stage\bin\ntctl" ./cmd/ntctl; if ($LASTEXITCODE) { throw "go build ntctl lỗi" }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Pop-Location

# File chữ: đổi xuống dòng về LF cho Linux
$utf8 = New-Object Text.UTF8Encoding $false
function Lf($from, $to) { [IO.File]::WriteAllText($to, ([IO.File]::ReadAllText($from) -replace "`r`n", "`n"), $utf8) }
Lf "$root\deploy\server\api\Dockerfile" "$stage\Dockerfile"
Lf "$root\deploy\server\docker-compose.yml" "$stage\docker-compose.yml"
Get-ChildItem "$root\backend\migrations\*.sql" | ForEach-Object { Lf $_.FullName "$stage\migrations\$($_.Name)" }

Write-Host "== đóng gói (nén gzip: mạng tải lên chậm, gói nén nhỏ hơn ~3 lần)"
# gói = đúng cây thư mục api.new trên máy chủ: Dockerfile, bin/, migrations/, web/ (+ web/app/)
Copy-Item -Recurse "$root\frontend\dist" "$stage\web"
if ($withApp) { Copy-Item -Recurse "$app\build\web" "$stage\web\app" }
$pkg = Join-Path $env:TEMP "vtnt-deploy.tgz"
Remove-Item -Force $pkg -ErrorAction SilentlyContinue
# tar có sẵn của Windows (gọi từ Git Bash thì "tar.exe" là GNU tar, hiểu "C:" là máy chủ từ xa -> lỗi)
$tar = Join-Path $env:SystemRoot "System32\tar.exe"
& $tar -czf $pkg -C $stage Dockerfile bin migrations web; if ($LASTEXITCODE) { throw "đóng gói lỗi" }
Write-Host ("   gói {0:N1} MB" -f ((Get-Item $pkg).Length / 1MB))

Write-Host "== tải lên $HostName`:$Dir"
scp -q $pkg "${HostName}:/tmp/vtnt-deploy.tgz"; if ($LASTEXITCODE) { throw "tải gói lên lỗi" }
ssh $HostName "set -e; rm -rf $Dir/api.new && mkdir -p $Dir/api.new && tar -xzf /tmp/vtnt-deploy.tgz -C $Dir/api.new && rm -f /tmp/vtnt-deploy.tgz"; if ($LASTEXITCODE) { throw "giải nén trên máy chủ lỗi" }
scp -q "$stage\docker-compose.yml" "${HostName}:$Dir/docker-compose.yml"

Write-Host "== dựng image, chạy migration, khởi động lại"
ssh $HostName "set -e; cd $Dir; rm -rf api.old; [ -d api ] && mv api api.old; mv api.new api; if [ ! -d api/web/app ] && [ -d api.old/web/app ]; then cp -r api.old/web/app api/web/app; echo 'giu ban /app/ dang chay'; fi; chmod +x api/bin/*; docker compose build -q api; for f in api/migrations/*.sql; do docker exec -i vtnt-db psql -q -v ON_ERROR_STOP=1 --single-transaction -U vuathau -d vuathau < `$f; done; docker compose up -d api; sleep 8; docker inspect -f 'vtnt-api {{.State.Health.Status}}' vtnt-api; curl -s -w ' [%{http_code}]\n' http://127.0.0.1:8095/health"
if ($LASTEXITCODE) { throw "Bước trên máy chủ lỗi (migration / khởi động). API cũ vẫn chạy; thư mục api đã đổi sang bản mới, bản trước ở $Dir/api.old." }
Write-Host "Xong. Bản trước nằm ở $Dir/api.old (quay lại: mv api.old api && docker compose build api && docker compose up -d api)"
