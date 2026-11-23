# Galaxy Terminal과 Galaxy IDE

갤럭시 탭에서 Ubuntu를 열고 코드를 편집·빌드할 수 있는 Android 터미널과 CLI 편집기입니다.

| 프로그램 | 다운로드 | 소스와 사용법 | 버전별 기록 |
| --- | --- | --- | --- |
| Galaxy Terminal (Android) | [0.4.11 APK](https://github.com/qqqqdh/Galaxy/releases/tag/terminal-v0.4.11) | [터미널 사용법과 빌드](terminal/) | [터미널 배포 기록](versions/terminal/) |
| Galaxy IDE (Ubuntu ARM64) | [0.1.16 실행 파일](https://github.com/qqqqdh/Galaxy/releases/tag/ide-v0.1.16) | [IDE 사용법과 빌드](cli-ide/) | [IDE 변경 이력](cli-ide/CHANGELOG.md) |

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
