# Galaxy IDE

Galaxy Terminal의 Ubuntu에서 실행하는 키보드·터치 지원 터미널 IDE입니다. 최신 버전은 **0.1.10**입니다.

- [Linux ARM64 다운로드](https://github.com/qqqqdh/Galaxy/releases/tag/ide-v0.1.10)
- [사용법과 소스 빌드](cli-ide/README.md)
- [버전별 변경 내용과 체크섬](versions/)
- [전체 변경 이력](cli-ide/CHANGELOG.md)
- [두 파일 C++ 예제](cli-ide/examples/clangd/)

## 설치

릴리스에서 `galaxy-ide-linux-arm64`를 받은 뒤 Ubuntu에서 실행하세요.

```sh
mkdir -p ~/.local/bin
cp galaxy-ide-linux-arm64 ~/.local/bin/galaxy-ide
chmod +x ~/.local/bin/galaxy-ide
sudo apt install clangd
galaxy-ide ~/project
```

C/C++ 편집 중 후보는 하단에 나타나며 Tab으로 적용합니다. 함수 이름 위에서 Ctrl+]를 누르면 정의가 있는 파일이 열립니다.

0.1.0부터 0.1.8까지의 태그는 배포 기록과 체크섬을 담습니다. 0.1.9와 0.1.10 태그에는 해당 시점의 IDE 소스도 포함되어 있습니다.
