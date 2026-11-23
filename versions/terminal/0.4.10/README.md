# Galaxy Terminal 0.4.10

- Codex 등 실행 중인 프로그램에서 Android 네이티브 입력창으로 한영 조합 범위와 커서를 관리합니다. 확정된 부분만 전송하고 조합 중인 글자를 미리 표시합니다.
- Enter를 포함한 연속 입력에서도 입력창의 문맥을 유지합니다. WebView 경로의 중복 확정 알림 방지도 보완했습니다.
- 터미널의 일반 URL과 OSC 8 링크를 클릭하면 내장 브라우저에서 바로 엽니다. 줄바꿈된 긴 URL과 localhost 주소도 지원합니다.

패키지 이름은 `dev.galaxy.terminal`, 버전 코드는 `16`입니다. [GitHub 릴리스](https://github.com/qqqqdh/Galaxy/releases/tag/terminal-v0.4.10)의 APK는 당시 소스를 기존 서명키로 다시 빌드한 파일입니다. 이전 로컬 빌드의 SHA-256은 `5fe754312551ce2189525cdb3e1be5800a5b01ed6c2c1c2f92b20b5803702e84`이며, 함께 제공하는 `SHA256SUMS`는 GitHub에 올린 재빌드본의 값입니다.

검증: Chrome 터미널 회귀 검사, 브라우저 주소 검증, APK 빌드·서명 검증을 통과했습니다. 연결된 Galaxy Tab S8+의 삼성 입력기에서도 별도 테스트 앱으로 일반 버퍼 50회·대체 버퍼 50회의 한영 전환·연속 입력·Enter를 검사해 중복·누락 없이 통과했습니다. 실제 Codex 대화와 로그인 웹사이트는 검사하지 않았습니다.

이후 실제 Codex 입력창을 터치한 상태에서는 한영 전환 중 중복과 자모 분리가 재현됐습니다. [0.4.11](https://github.com/qqqqdh/Galaxy/releases/tag/terminal-v0.4.11)에서 이 경로를 수정했습니다.
