# Galaxy IDE 0.1.20

Claude Code에 일반 문자나 백스페이스를 보낼 때, Claude가 입력줄을 갱신하기 전 IDE 전체를 다시 그리던 동작을 건너뜁니다. 빠른 한글 입력에서도 조합이 실제 입력줄에 바로 반영됩니다.

실행 파일은 Ubuntu ARM64용 `galaxy-ide-linux-arm64`입니다. [설치 및 사용법](../../cli-ide/README.md)

Galaxy Tab S8+의 Claude Code 입력창에 ADB로 한글 키 72개를 약 1.1초에 전달했습니다. 첫 캡처에서 `한글` 12회가 모두 표시됐으며, 추가 대기 후에도 글자가 바뀌거나 늘어나지 않았습니다.
