# xFile_search

대량의 파일에서도 빠르게 파일명/경로를 찾을 수 있도록 만든 Windows용 초고속 파일 검색 프로그램입니다. Everything처럼 단순하고 빠르게 사용하는 것을 목표로 하되, Preview와 키보드 중심 작업 기능을 강화하고 있습니다.

현재 버전: **0.1.26**


## 설치 (권장)

일반 Windows 사용자는 GitHub **Releases**에서 **`xFile_search_Setup_v0.1.26_x64.exe`** 하나만 다운로드해 실행하면 됩니다. 관리자 권한 없이 사용자 계정에 설치되며, 업데이트할 때 기존 Index와 검색 기록을 보존합니다.

무설치 사용자는 **`xFile_search_Portable_v0.1.26_x64.zip`**을 받아 압축 해제 후 `xFile_search.exe`를 실행하면 됩니다. 자세한 내용은 [INSTALL_KO.md](INSTALL_KO.md)를 참고하세요.

## 주요 기능

- 대량 파일 인덱스 기반의 빠른 파일 검색
- `*.jpg`, `D:\\*.jpg`, 특정 폴더 범위 검색 지원
- **Search Within**으로 현재 검색 결과 안에서 추가 검색
- 검색 결과는 먼저 빠르게 표시하고 Size/Date 등 메타데이터는 background에서 보충
- 파일 목록: **Name | Path | Size | Date**
- 각 헤더 클릭으로 오름차순/내림차순 정렬
- Preview를 켜고 폭을 조절해도 Size/Date가 보이도록 Name/Path 폭 자동 조절
- 이미지 Preview: 마우스 위치 기준 휠 확대/축소, 드래그 Pan, **1:1**, **Fit Window**
- `↑/↓`로 파일 이동, `Enter`로 연결 프로그램 열기
- 이미지 `Del` → 확인 후 Windows 휴지통으로 이동, `Esc`로 취소
- `←/→`로 File List와 Preview 사이 포커스 이동
- 최근 검색 기록 저장 및 재사용
- 파일/폴더 오른쪽 클릭 시 Windows Explorer Shell context menu 표시
- 인덱스 폴더를 실행파일 근처에 두어 확인/백업하기 쉬운 Portable 구조

## 빌드 환경

- Windows 10/11 x64
- Go 1.23.2 이상

Windows에서 다음 파일을 실행합니다.

```bat
build_windows.bat
```

또는 직접 빌드할 수 있습니다.

```bat
go test ./...
go build -trimpath -ldflags="-H=windowsgui" -o xFile_search.exe .
copy /Y xFile_search.exe xFile_indexer.exe
```

## 실행 폴더 예시

```text
xFile_search/
├─ xFile_search.exe
├─ xFile_indexer.exe
├─ xFile_search.ini
├─ SearchHistory.txt
├─ Index/
├─ Logs/
└─ Backup/
```

`Index`, `Logs`, 검색 기록 등 실행 중 생성되는 데이터는 GitHub 소스에 올라가지 않도록 `.gitignore`에 제외했습니다.

## Preview 참고

이미지는 xFile_search 자체 Preview 기능을 사용합니다. Word / Excel / PowerPoint / PDF 등의 문서는 Windows에 등록된 Preview Handler의 설치 상태에 따라 미리보기 가능 여부가 달라질 수 있습니다.

## 보안

현재 보안 정리 구조에서는 PowerShell 실행, `ExecutionPolicy Bypass`, `rundll32` fallback, 숨김 브라우저 실행, 네트워크 다운로드 기능을 사용하지 않습니다. 자세한 내용은 [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md)를 참고하세요.

## 변경 기록

[CHANGELOG.md](CHANGELOG.md)를 참고하세요.

## License

아직 공개 라이선스를 선택하지 않았습니다. 라이선스를 추가하기 전에는 일반적인 저작권 제한이 적용됩니다.
