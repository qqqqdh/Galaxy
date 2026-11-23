# Galaxy Terminal

한글 입력과 마우스·트랙패드 조작을 지원하는 Android 터미널입니다. 앱을 열면 Android 기본 셸(`/system/bin/sh`)이 시작됩니다. 상단 **Ubuntu**를 누르면 앱 안에서 별도의 Ubuntu 24.04 LTS 셸을 실행할 수 있습니다.

## 설치와 사용

1. [Galaxy Terminal 0.4.10 릴리스](https://github.com/qqqqdh/Galaxy/releases/tag/terminal-v0.4.10)에서 `Galaxy-Terminal.apk`를 받아 Android 8 이상 기기에 설치합니다.
2. Galaxy Terminal을 열고 물리 키보드를 사용하거나, 상단 **키보드**를 눌러 한국어가 설정된 Android 키보드를 엽니다.
3. `echo '한글 입력 확인'`을 입력합니다. `pwd`로 현재 디렉터리, `stty size`로 터미널 크기를 확인할 수 있습니다.

- Android 셸과 Ubuntu Bash 프롬프트에는 한글 조합 중인 글자도 실시간으로 입력됩니다. `ㅎ → 하 → 한`처럼 바뀌고 확정된 글자는 한 번만 남습니다. 실행 중인 명령의 입력에는 확정된 글자를 보냅니다.
- 트랙패드로 두 손가락 스크롤하면 이전 출력을 볼 수 있습니다. 클릭한 채 드래그해 텍스트를 선택하고, 우클릭 메뉴 또는 상단 **⋯**에서 복사하거나 붙여넣을 수 있습니다.
- 물리 키보드의 복사와 붙여넣기는 `Ctrl+Shift+C`와 `Ctrl+Shift+V`입니다. `Ctrl+C`는 실행 중인 명령에 인터럽트를 보냅니다. 화면 아래의 **Ctrl**은 다음 한 글자에 적용됩니다.
- `exit`로 셸을 종료하면 **새 셸** 버튼이 나타납니다. 작업 파일은 앱의 내부 저장공간에 저장됩니다. 앱을 삭제하거나 데이터를 지우면 파일도 삭제됩니다.

## Ubuntu 사용

1. ARM64 Android 9 이상 기기에서 **Ubuntu**를 누릅니다. 첫 화면에서 이름, 사용자 이름, 컴퓨터 이름, Ubuntu 비밀번호, 언어·지역, 시간대를 선택합니다. 입력기는 Android에서 선택합니다.
2. 초기 설정에는 인터넷이 필요합니다. 앱이 공식 Ubuntu Base 이미지 약 30MB를 검증해 설치한 뒤 `sudo`와 로캘·시간대 패키지를 설치합니다. 설치가 끝나면 생성한 일반 계정의 홈 디렉터리에서 Bash가 열립니다. [Canonical Ubuntu Base](https://cdimage.ubuntu.com/ubuntu-base/releases/24.04.5/release/)
3. `cat /etc/os-release`로 버전을 확인합니다. 패키지를 설치하려면 `sudo apt update`와 `sudo apt install nano`처럼 실행합니다. `sudo`가 요청하면 초기 설정에서 정한 Ubuntu 비밀번호를 입력합니다.
4. **‹**로 돌아가면 Ubuntu 셸이 닫힙니다. 다음에 **Ubuntu**를 열면 같은 계정으로 시작합니다. 이전 root 전용 버전에서 업데이트할 때도 초기 설정 화면이 한 번 열립니다. 이전 패키지와 파일은 Ubuntu 안에 남으며, 기존 `/root` 작업은 `sudo`를 통해 접근할 수 있습니다.

Ubuntu는 앱에 포함된 PRoot로 실행됩니다. Termux, PC, Android 루팅은 필요 없습니다. 설정한 컴퓨터 이름은 Ubuntu의 `/etc/hostname`과 Bash 프롬프트에 적용됩니다. 계정과 `sudo` 권한의 범위는 Ubuntu 내부입니다. Android 커널을 공유하므로 이 설치에는 디스크 파티션이나 systemd 부팅 단계가 없습니다. [PRoot Android 실행 방식](https://github.com/oonid/pr/blob/7029b1fe8fa84d64339cabe38f9c5be2ea2ae7ac/docs/targetsdk35-compatibility.md)

Ubuntu 셸에서 `codex`를 실행하면 `--no-daemon`이 기본 적용됩니다. Android의 프로세스 시작 시간 조회 제한으로 발생하는 Codex 백그라운드 서버 시작 오류를 피하며, `codex resume`과 `codex fork`에도 적용됩니다.

## CLI IDE

Ubuntu에서 `galaxy-ide` 또는 `galaxy-ide 프로젝트폴더`로 CLI IDE를 실행합니다. 파일 탐색기·여러 파일 탭·검색과 바꾸기를 지원하며, `Ctrl+S`로 저장하고 `Ctrl+Q`로 종료합니다. `Ctrl+R`에서 프로젝트 명령과 Codex·Claude·AGY를 열고 화면 위 **도움말**에서 단축키를 확인합니다. 설치와 실행 작업 확장은 [CLI IDE 설명](../cli-ide/README.md)을 참고하세요.

Galaxy IDE 0.1.1부터 물리 키보드로 입력하면 편집 본문에서 `ㅎ → 하 → 한`이 같은 자리에서 바뀌고 커서가 따라갑니다. 선택 영역을 바꿀 때는 확정 전까지 원래 선택을 보존합니다.

Ubuntu 셸과 Galaxy IDE 하단 터미널은 별도의 TTY로 로그인 목록에 표시됩니다. `who` 또는 `w`로 확인하며, 셸에서 `exit`하거나 Ubuntu 창을 닫으면 해당 세션은 목록에서 빠집니다. Ubuntu 24.04의 procps-ng 4.0.4에서는 로컬 세션을 표시할 때 오류가 나는 원격 호스트(FROM) 열을 생략합니다.

## 내장 브라우저

터미널 상단의 **브라우저**를 눌러 웹 주소나 검색어를 입력합니다. 검색어는 Google 검색으로 열립니다. 일반 사이트는 HTTPS로 열리고, 기기에서 실행 중인 서버는 `http://127.0.0.1:8080` 또는 `localhost:8080`처럼 입력할 수 있습니다. 서버가 다른 포트를 사용한다면 주소의 포트를 바꾸세요. 브라우저에서 **터미널**을 누르면 실행 중이던 셸로 돌아갑니다.

0.4.10부터 터미널 출력의 URL을 클릭하면 해당 주소가 내장 브라우저에서 바로 열립니다. 긴 주소가 여러 줄에 걸쳐 표시된 경우와 OSC 8 링크도 지원합니다. HTTPS와 기기 내부 HTTP 주소에 기존 브라우저의 주소 검증 규칙을 동일하게 적용합니다.

## 0.4.10 변경

- Codex 등 실행 중인 프로그램의 입력에 기존 IDE용 Android 입력창을 확장 적용했습니다. Android가 조합 범위와 커서를 관리하고, 확정된 부분만 터미널에 보내며 조합 중인 글자는 커서 옆에 미리 표시합니다. Enter도 같은 입력창에서 처리해 빠른 한영 전환과 연속 입력 중 문맥이 끊기지 않도록 했습니다.
- WebView 입력 경로에서도 조합 확정 알림과 동일한 문자·입력값이 다시 들어오면 재전송하지 않습니다. 실제로 반복 입력한 글자는 유지합니다.
- 터미널 링크를 클릭하면 내장 브라우저로 주소를 전달합니다.

## Windows 빌드

PowerShell 7, JDK, Node.js, Android SDK API 35, Build Tools 35.0.0, NDK 28.2.13676358을 사용합니다. 저장소 루트의 `.build-cache/android`를 SDK 경로로 사용합니다. Android SDK Command-line Tools가 PATH에 있다면 아래 명령으로 필요한 구성 요소를 설치할 수 있습니다.

```powershell
sdkmanager --sdk_root=.build-cache/android "platforms;android-35" "build-tools;35.0.0" "ndk;28.2.13676358"
npm --prefix terminal ci --ignore-scripts
./terminal/build.ps1
```

빌드 결과는 `dist/Galaxy-Terminal.apk`입니다. 최초 빌드는 SHA-256으로 고정된 PRoot 실행기와 소스를 GitHub에서 받아 APK에 포함합니다. 업데이트 설치가 필요하면 `.build-cache/terminal/terminal-dev.jks`를 보관하세요. `node terminal/smoke.cjs`로 화면과 키 입력을 검사할 수 있습니다. 기존 Galaxy Code의 Playwright 캐시와 Microsoft Edge를 사용합니다.

Edge 대신 Chrome으로 검사하려면 `$env:TERMINAL_TEST_BROWSER = 'chrome'`을 설정한 뒤 같은 검사 명령을 실행합니다.
