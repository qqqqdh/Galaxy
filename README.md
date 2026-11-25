# Galaxy Terminal과 Ubuntu 개발 도구

갤럭시 탭에서 Ubuntu를 열고 코드를 편집·빌드할 수 있는 Android 터미널과 Ubuntu 개발 도구입니다.

| 프로그램 | 공식 다운로드 | 소스와 사용법 | 버전별 기록 |
| --- | --- | --- | --- |
| Galaxy Terminal (Android) | [0.5.0 APK](https://github.com/qqqqdh/Galaxy/releases/tag/terminal-v0.5.0) | [터미널 사용법과 빌드](terminal/) | [터미널 배포 기록](versions/terminal/) |
| Galaxy IDE (Ubuntu ARM64) | [0.2.1 실행 파일](https://github.com/qqqqdh/Galaxy/releases/tag/ide-v0.2.1) | [IDE 사용법과 빌드](cli-ide/) | [IDE 변경 이력](cli-ide/CHANGELOG.md) |
| Galaxy 파일 관리자 (Ubuntu) | [0.1.2 실행 파일](https://github.com/qqqqdh/Galaxy/releases/tag/file-manager-v0.1.2) | [파일 관리자](file-manager/) | [파일 관리자 버전 기록](versions/file-manager/) |
| Galaxy 서버 실행 관리자 (Ubuntu) | [0.1.1 실행 파일](https://github.com/qqqqdh/Galaxy/releases/tag/server-manager-v0.1.1) | [서버 실행 관리자](server-manager/) | [서버 실행 관리자 버전 기록](versions/server-manager/) |

## 최신 릴리스 변경 사항

- Galaxy Terminal은 Ubuntu 셸 탭, 내장 브라우저의 페이지 상태 유지와 가로 분할 화면을 지원합니다. Ubuntu에 `galaxy-open`, `galaxy-share`, `galaxy-doctor`가 포함됩니다.
- Galaxy Terminal 0.5.0은 백그라운드 셸의 화면 갱신을 쉬게 하고 탭 전환과 트랙패드 스크롤 반응을 개선합니다. Galaxy IDE 0.2.1은 셸 출력 해석과 편집기 화면 갱신을 분리합니다.
- Galaxy IDE에 명령 팔레트, 줄 번호와 오류 이동, 참조 검색과 이름 변경, 현재 Go 테스트 실행을 추가했습니다. 웹 미리보기 자동 갱신, CSS 변수 색상 미리보기와 터치 편집, 프로젝트 작업 위치 복원을 지원합니다.
- Galaxy 파일 관리자는 휴지통과 프로젝트 백업을 지원합니다. 파일·설정·Git 기록을 Android에 저장하고 새 폴더에 복원할 수 있습니다.
- Galaxy 서버 실행 관리자는 프로세스와 TCP 포트 연결 상태를 따로 표시합니다. 최근 로그에 포트 충돌 메시지가 있으면 포트 상태에 표시합니다.

## Galaxy Terminal 설치

Android에서 릴리스의 `Galaxy-Terminal.apk`를 설치하세요. 앱을 열면 Android 셸이 시작됩니다. ARM64 Android 9 이상 기기에서는 상단 **Ubuntu**를 눌러 Ubuntu 24.04 LTS 계정을 만들 수 있습니다. 초기 설정에는 인터넷 연결이 필요합니다.

## Galaxy IDE 설치

릴리스에서 `galaxy-ide-linux-arm64`를 받은 뒤 Ubuntu에서 실행하세요.

```sh
mkdir -p ~/.local/bin
cp galaxy-ide-linux-arm64 ~/.local/bin/galaxy-ide
chmod +x ~/.local/bin/galaxy-ide
sudo apt install clangd
galaxy-ide ~/project
```

[IDE 0.1.11 소스와 변경 기록](versions/0.1.11/)에는 Java, JavaScript, HTML, CSS, Go, Python, Rust 지원이 추가됐습니다. 언어 서버가 제공하는 후보는 하단에 나타나며 Tab으로 적용합니다. 함수 이름 위에서 Ctrl+]를 누르면 정의가 있는 파일이 열립니다.

0.1.0부터 0.1.8까지의 태그는 배포 기록과 체크섬을 담습니다. 0.1.9부터는 태그에 해당 시점의 IDE 소스도 포함되어 있습니다.
