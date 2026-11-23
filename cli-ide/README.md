# Galaxy IDE

Galaxy Terminal의 Ubuntu에서 실행하는 CLI IDE입니다. 파일을 열고 바로 입력하며, 파일 탐색기·여러 탭·검색과 바꾸기·프로젝트 명령 실행을 지원합니다. 마우스와 터치패드로 파일과 메뉴를 선택할 수 있습니다. 편집기 실행에는 Python, Node.js, PC 연결이 필요하지 않습니다.

```bash
galaxy-ide
galaxy-ide ~/ice-website-redesign-archive
galaxy-ide main.py
```

폴더를 지정하면 그 폴더가 프로젝트 기준 경로가 됩니다. 파일을 지정하면 해당 파일의 상위 폴더를 기준으로 엽니다. 새 경로를 지정하면 빈 파일로 열고, 저장할 때 파일과 상위 폴더를 만듭니다.

## 설치

ARM64 Ubuntu에 `dist/galaxy-ide-linux-arm64`를 옮기고 다음 명령으로 설치합니다. 배포 파일은 실행에 필요한 Go 라이브러리를 포함합니다.

```bash
mkdir -p ~/.local/bin
cat galaxy-ide-linux-arm64 > ~/.local/bin/galaxy-ide
chmod 755 ~/.local/bin/galaxy-ide
galaxy-ide --version
```

저장소 전체를 받았다면 `bash cli-ide/install.sh`도 사용할 수 있습니다. `~/.local/bin`이 PATH에 있어야 `galaxy-ide`로 실행됩니다. Galaxy Terminal의 기존 CLI 설치 환경에는 이 경로가 포함돼 있습니다.

## 사용

C/C++, Java, JavaScript, HTML, CSS, Go, Python, Rust, 셸 등은 파일 이름으로 언어를 판별해 문법을 강조합니다. 새 문서는 `main.c`처럼 이름을 붙여 저장하면 바로 색이 적용됩니다. 일반 텍스트는 기본 글자색으로 표시합니다.

| 키 | 동작 |
|---|---|
| Ctrl+O | 경로로 파일 열기 · 새 경로로 파일 만들기 |
| Ctrl+P | 파일명이나 상대 경로의 일부 입력 · ↑↓로 선택 후 Enter 또는 터치로 열기 |
| Ctrl+N / Ctrl+S / Ctrl+W | 새 파일 / 저장 / 현재 탭 닫기 |
| Ctrl+Q | 종료 · 미저장 변경 확인 |
| Ctrl+A | 전체 선택 |
| Shift+방향키 | 텍스트 선택 |
| Ctrl+C / Ctrl+X / Ctrl+V | IDE 내부 복사 / 잘라내기 / 붙여넣기 |
| Ctrl+Z / Ctrl+Y | 실행 취소 / 다시 실행 |
| Ctrl+F / F3 | 현재 파일 찾기 / 다음 결과 |
| Ctrl+H / Ctrl+G | 바꾸기 / 행 이동 |
| Tab / Enter | 들여쓰기 / 자동 들여쓰기 |
| F1 / F2 | 도움말 / 파일 탐색기 표시 전환 |
| F4 / Ctrl+B | 다음 탭 / 탐색기와 편집기 사이 이동 |
| Ctrl+R | 프로젝트의 `build` 작업 실행 · 미지정 시 현재 파일 실행·웹 미리보기 또는 실행 메뉴 |
| F5 / Ctrl+T | 실행 작업 메뉴 / 프로젝트 셸 |
| Ctrl+E | 파일 메뉴 · 다른 이름으로 저장 · 설정 |

Android 클립보드의 텍스트를 넣으려면 터미널의 붙여넣기 메뉴 또는 `Ctrl+Shift+V`를 사용합니다. IDE 내부 클립보드는 열린 탭 사이에서 공유합니다. 상단 **탐색기** 버튼으로 파일 목록을 접거나 다시 엽니다. 폭이 아주 좁으면 이 버튼이 빠른 파일 열기를 엽니다.

하단 **검색**에서 문구를 입력하고 Enter를 누르면 프로젝트의 UTF-8 텍스트 파일을 검색합니다. 열린 파일은 현재 편집 내용으로 검색하며, 이름 없는 새 문서도 포함합니다. 결과를 누르거나 ↓로 목록에 포커스를 옮겨 Enter를 누르면 해당 줄의 문구가 선택됩니다. 본문 검색은 대소문자를 구분하며 한 줄당 한 건, 최대 500줄을 표시합니다. 빠른 파일 열기는 대소문자를 구분하지 않으며 최대 100개 후보를 표시합니다. 두 기능 모두 프로젝트의 `ignore` 설정과 `.git`, `build`, `cmake-build-*`를 제외하고 심볼릭 링크를 따라가지 않습니다.

C/C++ 기본 빌드는 프로젝트 폴더와 하위 폴더의 소스를 컴파일하고 실행 파일 하나로 링크합니다. 예를 들어 `galaxy-ide ~/calculator`로 열면 `main.c`와 `src/add.c` 중 어느 파일에서든 두 파일을 함께 빌드합니다. C와 C++를 함께 쓰면 각 파일의 언어로 컴파일하고 C++로 링크합니다. 헤더 검색 경로에는 프로젝트 폴더와 그 아래 `include/`가 들어갑니다.

프로그램 하나당 프로젝트 폴더 하나를 사용하세요. 숨김 경로, `build/`, `cmake-build-*/`, 설정의 `ignore` 항목은 자동 탐색에서 제외합니다. 홈이나 파일 시스템 최상위에서 IDE를 시작하거나 프로젝트 밖의 파일을 열면 해당 소스의 상위 폴더를 기준으로 합니다. 그 폴더 자체가 홈이나 파일 시스템 최상위면 바로 아래의 파일만 모읍니다.

빌드 범위 안의 열린 파일에 미저장 변경이 있으면 저장 여부를 묻습니다. 빌드에 성공하면 프로그램을 실행하고, 실패하면 컴파일러나 링커의 진단을 표시합니다. 프로그램에는 터미널에서 입력할 수 있고 실행이 끝나면 Enter로 편집기에 돌아옵니다. 실행 파일은 임시 폴더에 만들고 실행 후 정리합니다.

하단 **문제** 탭(Ctrl+J)은 C/C++에서 입력을 잠시 멈추면 미저장 내용도 GCC로 검사합니다. 오류는 빨간 밑줄, 경고는 노란 밑줄로 표시되며 목록에서 Enter를 누르면 해당 위치로 이동합니다. Ctrl+R 빌드 후에는 프로젝트 전체의 컴파일·링크 메시지를 확인할 수 있습니다.

상단 **AI** 메뉴에서 Codex, Claude Code, AGY를 선택하면 파일 탐색기와 코드 옆에서 실행됩니다. 화면 폭이 좁으면 코드 아래에 AI 창이 열립니다. 코드나 AI 창을 터치하거나 `Ctrl+T`를 눌러 입력 위치를 전환합니다. **숨기기**로 AI 창을 접어도 세션은 유지되며, 상단 **AI → 대화 보기**로 다시 엽니다. 도구의 설치와 로그인은 Ubuntu 환경에서 준비해야 합니다. Codex에는 `--no-daemon`과 빠른 입력 묶기 해제를 적용합니다.

하단 프로젝트 터미널에서 이미 실행한 명령은 **터미널 확대** 또는 F8로 전체 화면에서 이어서 봅니다. **편집기로** 또는 Ctrl+T를 누르면 돌아가며, 셸과 실행 중인 도구는 계속 유지됩니다. F7은 별도 전체 화면 셸을 엽니다. AI 창에서 편집기로 옮기거나 AI가 종료되면, 외부에서 수정된 파일 중 편집기에 미저장 변경이 없는 파일을 다시 읽습니다.

실행·셸·탐색기·파일 메뉴에는 F5·F7·F6·F9도 사용할 수 있습니다. 삼성 키보드가 기능 키를 자체 동작으로 처리하면 Ctrl 조합이나 화면 위의 메뉴 버튼을 사용하세요.

## Git 변경 비교

하단 **변경사항** 탭을 누르면 Git 저장소의 변경 목록을 갱신합니다. 파일을 터치하거나 Enter를 누르면 삭제한 줄은 빨간색 `-`, 추가한 줄은 초록색 `+`로 표시합니다. **작업 트리**는 마지막으로 저장한 파일과 인덱스를, **스테이징**은 인덱스와 마지막 커밋을 비교합니다. 두 곳에서 모두 바뀐 파일은 각각 나타납니다. 추적하지 않은 파일의 내용도 확인할 수 있습니다. **목록으로** 또는 Esc를 누르면 목록으로 돌아갑니다.

## 실행 작업 확장

`F9`의 **프로젝트 설정 열기**에서 `.galaxy-ide.json`을 편집합니다. 저장하면 설정이 적용됩니다. 공통 설정은 `~/.config/galaxy-ide/config.json`에 두며, 프로젝트에 지정한 항목이 우선합니다. `tasks`와 `ignore` 배열은 상위 설정의 해당 배열을 대체합니다.

```json
{
  "indent_size": 4,
  "use_tabs": false,
  "tasks": [
    {"name": "Make 빌드 및 실행", "group": "build", "args": ["make", "run"]},
    {"name": "Make 테스트", "group": "test", "args": ["make", "test"]},
    {"name": "현재 파일 실행", "args": ["python3", "{file}"]},
    {"name": "개발 서버", "args": ["npm", "run", "dev"]},
    {"name": "Go 테스트", "args": ["go", "test", "./..."]}
  ]
}
```

`group`은 선택 사항입니다. 프로젝트당 `build`와 `test`를 각각 한 작업에 지정할 수 있습니다. 어느 탭에서든 편집기에서 `Ctrl+R`을 누르면 지정한 빌드를 실행합니다. 상단 **실행** 메뉴에는 지정한 빌드와 테스트가 먼저 표시됩니다. 실행 전에는 프로젝트 안에 열린 파일의 미저장 변경을 확인합니다.

위 예시의 `run`과 `test`는 해당 Makefile에 정의한 타깃입니다. CMake 프로젝트라면 이미 구성한 빌드 디렉터리에 맞춰 `["cmake", "--build", "build"]`와 `["ctest", "--test-dir", "build", "--output-on-failure"]`로 바꿀 수 있습니다.

`args`는 실행 파일과 인수를 구분한 배열입니다. `{file}`에는 저장한 파일의 절대 경로, `{root}`에는 프로젝트 폴더가 들어갑니다. 공백이 있는 파일명도 하나의 인수로 전달합니다. 파이프나 `&&`가 필요하면 `["bash", "-lc", "명령"]`처럼 셸을 명시합니다.

Go 소스는 저장 처리 `document.go`, 설정과 인수 치환 `config.go`, 화면과 단축키 `app.go`, 대화상자 `dialogs.go`, 외부 도구 실행 `tasks.go`로 나뉩니다. 실행 작업을 추가하는 데 IDE 재빌드는 필요하지 않습니다.

## 저장과 현재 지원 범위

UTF-8 텍스트 파일을 8 MiB까지 편집합니다. 한글, UTF-8 BOM, LF/CRLF와 파일 권한을 보존합니다. 혼합된 줄바꿈은 CRLF가 하나라도 있으면 저장 시 CRLF로 통일됩니다. 심볼릭 링크는 연결 대상을 저장합니다. 파일은 같은 디렉터리의 임시 파일을 동기화한 후 교체합니다. 외부 변경이나 기존 파일 덮어쓰기는 확인 창을 거칩니다.

편집 내용이 바뀔 때마다 사용자 전용 복구 파일에 기록합니다. 앱이 강제 종료되면 Ubuntu에서 `galaxy-ide`를 다시 실행하세요. 이름 없는 새 문서를 포함한 미저장 문서가 편집 탭으로 돌아옵니다. 원본 파일에 반영하려면 Ctrl+S를 누르세요. 저장하거나 문서를 닫으며 ‘버리기’를 선택하면 복구 기록도 정리됩니다. 기본 위치는 `~/.local/state/galaxy-ide/drafts`이며 `XDG_STATE_HOME` 설정을 따릅니다. 앱 데이터 삭제나 기기 분실에 대비한 외부 백업은 별도로 필요합니다.

## 언어별 실행과 코드 지원

프로젝트에 `build` 작업을 지정하면 Ctrl+R은 그 작업을 우선합니다. 기본 작업은 다음과 같습니다.

| 파일 | Ctrl+R 기본 동작 | Ubuntu의 언어 서버 |
|---|---|---|
| Java (.java) | JDK의 `java 파일.java`로 실행 | `jdtls` |
| JavaScript (.js, .mjs, .cjs) | Node.js로 실행 | `typescript-language-server` |
| HTML (.html, .htm) | 프로젝트 로컬 서버 시작, 표시된 URL을 내장 브라우저로 열기 | `vscode-html-language-server` |
| CSS (.css) | 로컬 서버에서 가장 가까운 상위 index.html을 미리보기 (없으면 프로젝트 파일 목록) | `vscode-css-language-server` |
| Go (.go) | `go run 파일.go` | `gopls` |
| Python (.py, .pyw) | `python3 파일.py` | `pyright-langserver` |
| Rust (.rs) | rustc로 임시 실행 파일을 만들어 실행 | `rust-analyzer` |
| C/C++ | 프로젝트 소스를 함께 빌드하고 실행 | `clangd` |

색상 강조는 기본 제공됩니다. 자동완성(Ctrl+K, Tab으로 적용), 정의 이동(Ctrl+] 또는 F12), 문제 탭과 편집기 밑줄에는 언어 서버가 제공하는 결과가 표시됩니다. 서버에서 진단이나 정의 위치를 제공하는 범위는 언어와 프로젝트 설정에 따라 다릅니다. HTML/CSS 미리보기에는 Python 3이 필요하며, 서버는 기기의 루프백 주소에만 열립니다. Ctrl+C로 미리보기를 끝낸 뒤 Enter를 누르면 편집기로 돌아옵니다. Go 모듈 전체, Cargo, Maven, Gradle 프로젝트는 아래 예처럼 `.galaxy-ide.json`의 build 작업으로 실행할 수 있습니다.

```json
{"tasks":[{"name":"Cargo 실행","group":"build","args":["cargo","run"]}]}
```

### Ubuntu에서 언어 도구 준비

- Java는 JDK와 Eclipse JDT LS가 필요합니다. 갤럭시 탭의 ARM64 Ubuntu에서는 OpenJDK 21과 JDT LS 1.50.0을 확인했습니다. PRoot에서 tar가 JAR 파일을 디렉터리로 취급하면 새 폴더에 `python3 -m tarfile -e 서버파일.tar.gz 대상폴더`로 압축을 푸세요. 사용 중인 JDK와 호환되는 릴리스의 `bin/jdtls`를 PATH에 추가하세요. [Eclipse JDT LS 배포 안내](https://github.com/eclipse-jdtls/eclipse.jdt.ls#installation)
- JavaScript·HTML·CSS·Python 서버는 Node와 npm을 준비한 뒤 `npm install --global --prefix "$HOME/.local" typescript typescript-language-server vscode-langservers-extracted pyright`로 설치할 수 있습니다. Python 프로그램을 실행할 때는 `python3`도 필요합니다.
- Go는 Ubuntu에서 `sudo apt install golang-go gopls`로 설치합니다. Rust는 rustup의 stable 도구 모음에서 `rustup component add rust-analyzer rust-src`로 서버와 표준 라이브러리 소스를 추가합니다.
- Ubuntu의 PATH에 `~/.local/bin`과 사용하는 도구의 bin 폴더를 포함하고, 실행 중인 IDE를 다시 여세요.

### C/C++ 프로젝트 인덱싱

Ubuntu에서 `sudo apt install clangd`로 언어 서버를 설치한 뒤 프로젝트 폴더를 `galaxy-ide`로 여세요. C/C++ 파일에서 식별자 두 글자를 입력하거나 `.`, `->`, `::`를 입력하고 잠시 멈추면 하단에 후보가 뜹니다. 편집기에서 Tab을 누르면 첫 후보가 적용됩니다. ↓로 목록에 들어가 원하는 항목에서 Enter를 누르거나 항목을 터치할 수도 있습니다. Esc는 목록을 닫습니다.

`Ctrl+K`로 직접 후보를 요청할 수 있습니다. 목록은 커서 아래에 표시되며, 아래 공간이 부족하면 위에 표시됩니다. 위·아래 방향키로 후보를 고르고 Tab으로 적용하거나 후보를 클릭합니다. Esc로 닫습니다. 함수 이름에 커서를 두고 `Ctrl+]` 또는 F12를 누르면 정의가 있는 파일과 줄로 이동합니다. 물리 키보드가 단축키를 가로채면 상단 **파일** 메뉴의 **코드 자동완성**과 **함수 정의로 이동**을 사용하세요.

clangd에는 열린 C/C++ 문서의 미저장 내용이 전달됩니다. 프로젝트의 컴파일 데이터베이스를 사용하며, 전용 프로젝트 폴더에 별도 설정이 없으면 소스와 `include/` 기준으로 임시 데이터베이스를 생성합니다. 매크로, 외부 라이브러리, 개별 컴파일 옵션을 쓰는 프로젝트는 실제 빌드에서 생성한 `compile_commands.json`을 사용하세요. 첫 인덱싱 중에는 파일 간 정의 이동 결과가 늦게 준비될 수 있습니다.

## 빌드와 검사

Go 1.24.2 이상에서 빌드합니다. 라이브러리는 `go.mod`와 `go.sum`으로 고정합니다. UI는 [tview](https://github.com/rivo/tview), 터미널 입력은 [tcell](https://github.com/gdamore/tcell)을 사용합니다.

```powershell
pwsh -NoProfile -File cli-ide/build.ps1
```

Linux의 직접 빌드와 검사는 다음과 같습니다.

```bash
cd cli-ide
go test -timeout 45s ./...
go build -o galaxy-ide .
```
