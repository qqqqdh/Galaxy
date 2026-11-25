# Galaxy 서버 실행 관리자

Galaxy Terminal의 Ubuntu 셸에서 프로젝트별 개발 서버를 시작·중지하고 로그를 확인합니다. 목록과 버튼은 터치나 키보드로 선택할 수 있습니다.

## 설치

[0.1.1 릴리스](https://github.com/qqqqdh/Galaxy/releases/tag/server-manager-v0.1.1)에서 갤럭시 탭용 `galaxy-servers-linux-arm64`를 받으세요. Linux x86-64에서는 `galaxy-servers-linux-amd64`를 받습니다.

```bash
mkdir -p ~/.local/bin
cp galaxy-servers-linux-arm64 ~/.local/bin/galaxy-servers
chmod +x ~/.local/bin/galaxy-servers
galaxy-servers --version
galaxy-servers ~/project
```

Ubuntu 셸에서 명령을 찾지 못하면 `export PATH="$HOME/.local/bin:$PATH"`를 실행하세요.

## 사용법

**추가**를 눌러 이름, 실행 명령, 작업 폴더, 브라우저 주소를 저장합니다. 예를 들어 웹 프로젝트를 작업 폴더로 지정하고 실행 명령에 `npm run dev`, 주소에 `http://127.0.0.1:3000`을 입력합니다. 명령은 지정한 폴더에서 Ubuntu의 `/bin/sh`로 실행됩니다.

등록한 서버를 선택해 **서버 시작** 또는 **서버 중지**를 누릅니다. 아래에 표준 출력과 오류가 표시됩니다. **로그** 버튼이나 Tab으로 로그에 포커스를 두고 스크롤할 수 있습니다. 주소를 누르면 Galaxy Terminal 내장 브라우저에서 열 수 있습니다. **프로세스**에는 실행 또는 종료 상태, **포트**에는 지정한 로컬 주소의 TCP 연결 상태가 표시됩니다. 최근 로그에서 포트 충돌 메시지를 찾으면 포트 옆에 표시합니다.

서버 목록은 프로젝트별로 저장합니다. 관리자 화면을 종료해도 같은 Ubuntu 세션에서는 서버가 계속 실행됩니다. 같은 프로젝트로 관리자를 다시 열어 중지할 수 있습니다. Ubuntu 셸이나 앱을 종료하면 그 세션에서 시작한 서버도 종료됩니다. 관리자에서 중지할 수 있도록 서버 명령은 포그라운드로 실행하세요.

## 빌드

저장소 루트에서 Go 1.24.2 이상으로 빌드합니다. 갤럭시 탭용 실행 파일은 `bash server-manager/build.sh arm64`로 만들고, Linux x86-64용은 `arm64` 대신 `amd64`를 지정합니다. 결과는 `dist/`에 저장됩니다.

[버전별 기록](../versions/server-manager/)
