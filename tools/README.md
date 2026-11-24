# Galaxy Ubuntu 도구

Galaxy Terminal의 Ubuntu 셸에서 파일 관리자와 서버 실행 관리자를 별도 명령으로 엽니다. 목록과 작업 버튼은 터치 또는 키보드로 선택할 수 있고, Esc로 메뉴를 닫거나 프로그램을 종료합니다.

## 빌드와 설치

Linux에서 Go 1.24.2 이상으로 빌드합니다. 인수를 생략하면 현재 Go 호스트의 아키텍처를 사용합니다. 갤럭시 탭으로 옮길 실행 파일을 PC에서 만들 때는 `bash tools/build.sh arm64`를 실행하세요.

```bash
bash tools/build.sh
bash tools/install.sh
galaxy-files ~/project
galaxy-servers ~/project
```

빌드 결과는 `dist/`에 있습니다. 갤럭시 탭의 Ubuntu로 ARM64 실행 파일을 옮겼다면, 저장소의 설치 스크립트에 실행 파일을 담은 폴더를 지정하거나 파일을 직접 설치할 수 있습니다.

```bash
mkdir -p ~/.local/bin
cp galaxy-files-linux-arm64 ~/.local/bin/galaxy-files
cp galaxy-servers-linux-arm64 ~/.local/bin/galaxy-servers
chmod +x ~/.local/bin/galaxy-files ~/.local/bin/galaxy-servers
```

Ubuntu 셸에서 명령을 찾지 못하면 `export PATH="$HOME/.local/bin:$PATH"`를 실행하세요.

## 파일 관리자

파일이나 폴더를 터치하거나 선택 후 Enter를 눌러 작업 목록을 엽니다. 복사 또는 이동을 선택한 뒤 대상 폴더에서 **여기에 붙여넣기**를 누르세요. 이름 변경, 새 폴더, 숨김 파일 표시, 영구 삭제도 **작업**에서 선택합니다. ZIP, TAR.GZ, TGZ는 새 폴더에 풀며, 같은 이름의 기존 항목은 덮어쓰지 않습니다. 압축 파일 안의 링크와 특수 파일은 지원하지 않습니다.

- **F2**로 선택한 폴더를 열고 **Backspace**로 상위 폴더로 이동합니다.
- **Ctrl+C**로 복사할 항목을 선택하고 **Ctrl+V**로 붙여넣습니다. **F5**로 목록을 갱신합니다.
- 서로 다른 파일 시스템에 옮길 때는 대상에 복사한 뒤 원본을 삭제하세요.

Android와 파일을 주고받으려면 이 기능이 포함된 Galaxy Terminal APK가 필요합니다. Ubuntu 상단 **⋯ → Android 파일 전송 → Android에서 가져오기**로 Android 파일 선택기를 열면 파일이 `/mnt/galaxy-transfer`로 들어갑니다. 파일 관리자의 **폴더 → Android 파일 전송**에서도 해당 폴더를 열 수 있습니다. 반대 방향은 파일을 전송 폴더에 복사하고 **Android에 저장하기**에서 파일과 저장 위치를 고르세요. 폴더를 전송하려면 먼저 압축 파일로 만드세요.

## 서버 실행 관리자

**추가**를 눌러 이름, 실행 명령, 작업 폴더, 브라우저 주소를 저장합니다. 예를 들어 작업 폴더를 웹 프로젝트로 지정하고 실행 명령에 `npm run dev`, 브라우저 주소에 프로젝트가 사용하는 `http://127.0.0.1:3000`을 입력합니다. 명령은 지정한 폴더에서 Ubuntu의 `/bin/sh`로 실행됩니다.

등록한 서버를 선택해 **서버 시작** 또는 **서버 중지**를 누릅니다. 아래에 표준 출력과 오류가 표시됩니다. **로그** 버튼이나 Tab으로 로그에 포커스를 두고 스크롤할 수 있습니다. 주소를 등록했다면 주소를 눌러 Galaxy Terminal 내장 브라우저에서 열 수 있습니다. **실행 중**은 프로세스 상태이며, 실제 접속 가능 여부는 서버 로그와 브라우저에서 확인하세요.

서버 목록은 프로젝트 경로별로 저장됩니다. 관리자 화면을 종료해도 같은 Ubuntu 세션에서는 서버가 계속 실행됩니다. 같은 프로젝트로 관리자를 다시 열어 중지할 수 있습니다. Ubuntu 셸이나 앱을 종료하면 그 세션이 시작한 서버도 종료됩니다. 관리자에서 중지할 수 있도록 서버 명령은 포그라운드로 실행하세요.
