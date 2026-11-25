# Galaxy 파일 관리자

Galaxy Terminal의 Ubuntu 셸에서 파일과 폴더를 터치나 키보드로 관리합니다.

## 설치

[0.1.2 릴리스](https://github.com/qqqqdh/Galaxy/releases/tag/file-manager-v0.1.2)에서 갤럭시 탭용 `galaxy-files-linux-arm64`를 받으세요. Linux x86-64에서는 `galaxy-files-linux-amd64`를 받습니다.

```bash
mkdir -p ~/.local/bin
cp galaxy-files-linux-arm64 ~/.local/bin/galaxy-files
chmod +x ~/.local/bin/galaxy-files
galaxy-files --version
galaxy-files ~/project
```

Ubuntu 셸에서 명령을 찾지 못하면 `export PATH="$HOME/.local/bin:$PATH"`를 실행하세요.

## 사용법

파일이나 폴더를 터치하거나 선택 후 Enter를 눌러 작업 목록을 엽니다. 복사 또는 이동을 선택한 뒤 대상 폴더에서 **여기에 붙여넣기**를 누르세요. 이름 변경, 새 폴더, 숨김 파일 표시도 **작업**에서 선택합니다. ZIP, TAR.GZ, TGZ는 새 폴더에 풀며 같은 이름의 기존 항목은 덮어쓰지 않습니다. GitHub 소스 TAR.GZ의 PAX 전역 헤더를 건너뜁니다.

- **휴지통으로 이동**은 항목을 보관합니다. 아래 **휴지통**에서 원래 위치에 복원하거나 영구 삭제할 수 있습니다. 원래 경로에 같은 이름의 항목이 있다면 이름을 바꾼 뒤 복원하세요.
- **프로젝트 백업**과 **현재 폴더 백업**은 숨김 파일과 폴더 안의 Git 기록을 포함한 `.galaxy-backup.tar.gz`를 만듭니다. **Android에 백업**을 선택하면 시스템 파일 선택기에서 저장 위치를 고릅니다. 백업을 다시 가져오고 **압축 풀기**를 선택하면 새 폴더에 복원합니다. 프로젝트 밖의 Git 기록이나 외부 파일을 가리키는 링크가 있는 경우 백업이 중단됩니다.

- **F2**로 선택한 폴더를 열고 **Backspace**로 상위 폴더로 이동합니다.
- **Ctrl+C**로 복사할 항목을 선택하고 **Ctrl+V**로 붙여넣습니다. **F5**로 목록을 갱신합니다.
- 서로 다른 파일 시스템에 옮길 때는 대상에 복사한 뒤 원본을 삭제하세요.

Android와 파일을 주고받으려면 파일 전송 기능을 지원하는 [Galaxy Terminal](../terminal/)이 필요합니다. Ubuntu 상단 **⋯ → Android 파일 전송 → Android에서 가져오기**를 선택하면 받은 파일이 `/mnt/galaxy-transfer`에 표시됩니다. 파일 관리자의 **폴더 → Android 파일 전송**에서도 열 수 있습니다. 반대 방향은 보낼 파일을 전송 폴더에 복사한 뒤 **Android에 저장하기**에서 고르세요. 폴더를 전송하려면 먼저 압축 파일로 만드세요.

## 빌드

저장소 루트에서 Go 1.24.2 이상으로 빌드합니다. 갤럭시 탭용 실행 파일은 `bash file-manager/build.sh arm64`로 만들고, Linux x86-64용은 `arm64` 대신 `amd64`를 지정합니다. 결과는 `dist/`에 저장됩니다.

[버전별 기록](../versions/file-manager/)
