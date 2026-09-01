# xFile_search 설치 방법

## 권장: Windows Setup

1. GitHub **Releases**에서 `xFile_search_Setup_v0.1.29_x64.exe`를 다운로드합니다.
2. Setup 파일을 더블클릭합니다.
3. 설치 확인 창에서 **Yes**를 누릅니다.
4. 설치가 끝나면 xFile_search가 자동 실행됩니다.

기본 설치 위치:

`%LOCALAPPDATA%\Programs\xFile_search`

관리자 권한은 필요하지 않습니다. 기존 버전 위에 설치하면 프로그램 파일만 업데이트하고 `Index`, `Logs`, `Backup`, `SearchHistory.txt`는 유지합니다.

## Portable 버전

`xFile_search_Portable_v0.1.29_x64.zip`을 원하는 폴더에 압축 해제한 뒤 `xFile_search.exe`를 실행합니다.

## 제거

Windows **설정 > 앱 > 설치된 앱 > xFile_search > 제거**를 사용합니다.

## 처음 실행

처음 실행하면 Index가 없을 경우 백그라운드 인덱싱이 시작될 수 있습니다. 기존 v3 Index가 있다면 그대로 재사용할 수 있습니다.


## v0.1.29 첫 실행 참고

v0.1.27 이하에서 업데이트한 경우, 고정/이동식 드라이브의 Windows 볼륨 식별값을 처음 저장하기 위해 **한 번 자동 백그라운드 재인덱싱**이 실행될 수 있습니다.

인덱싱 중에도 프로그램은 계속 사용할 수 있습니다. 첫 번째 부분 인덱스가 준비되면 그 내용부터 바로 검색할 수 있고, 전체 인덱싱은 뒤에서 계속 진행됩니다. 하단에는 **굵은 초록색 `INDEXING... xx%`**가 표시되고, 창 제목, Reindex 버튼, Marquee 진행 표시, 상태 문구에서도 진행 여부를 확인할 수 있습니다. 퍼센트는 인덱싱 속도를 떨어뜨리는 사전 전체 파일-count를 하지 않기 위해 이전 완료 Index의 항목 수를 기준으로 계산하는 가벼운 추정치이며, 완료 전에는 최대 99%, 완료 시 100%로 표시됩니다.
