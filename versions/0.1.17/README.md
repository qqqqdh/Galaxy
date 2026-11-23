# Galaxy IDE 0.1.17

- Claude Code 인증 링크가 좁은 AI 창에서 여러 줄로 표시돼도 링크를 누르면 전체 주소를 엽니다. 주소가 잘려 `redirect_uri`가 누락되던 문제를 수정했습니다.
- Galaxy Terminal 0.4.12 이상은 이 인증 링크를 Android 브라우저에서 엽니다.

실행 파일은 Ubuntu ARM64용 `galaxy-ide-linux-arm64`입니다. [설치 및 사용법](../../cli-ide/README.md)

줄바꿈 전후의 인증 주소가 완전히 같은지 Go 테스트로 확인했습니다. Galaxy Tab S8+에 설치하고 Firefox의 인증 반환 페이지와 Claude Code 입력창을 확인했습니다.
