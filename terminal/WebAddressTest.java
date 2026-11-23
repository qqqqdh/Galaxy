package dev.galaxy.terminal;

import java.util.Objects;

public final class WebAddressTest {
    public static void main(String[] args) {
        expect(" example.com ", "https://example.com");
        expect("example.com/?next=https://other.example", "https://example.com/?next=https://other.example");
        expect("127.0.0.1:8080/page", "http://127.0.0.1:8080/page");
        expect("localhost:5173", "http://localhost:5173");
        expect("https://localhost:8443/", "https://localhost:8443/");
        expect("codex", "https://www.google.com/search?q=codex");
        expect("코덱스 사용법", "https://www.google.com/search?q=%EC%BD%94%EB%8D%B1%EC%8A%A4+%EC%82%AC%EC%9A%A9%EB%B2%95");
        expect("a & b", "https://www.google.com/search?q=a+%26+b");
        for (String invalid : new String[] { "", "http://example.com", "http://localhost.example.com",
                "http://localhost@external.example", "https://login@example.com", "localhost:65536",
                "javascript:alert(1)", "file:///etc/passwd", "intent://test", "https://host name" }) {
            expect(invalid, null);
        }
        if (!WebAddress.allowed("https://example.com") || !WebAddress.allowed("http://127.0.0.1:8080")
                || WebAddress.allowed("http://example.com") || WebAddress.allowed("content://example.com")) {
            throw new AssertionError("Unexpected navigation policy");
        }
        expectClaudeAuthorization("https://claude.com/cai/oauth/authorize?state=test", true);
        expectClaudeAuthorization("https://claude.ai/oauth/authorize?state=test", true);
        expectClaudeAuthorization("https://claude.com/", false);
        expectClaudeAuthorization("https://auth.openai.com/oauth/authorize", false);
        expectClaudeAuthorization("https://claude.com.example.org/cai/oauth/authorize", false);
        expectClaudeAuthorization("https://claude.com:8443/cai/oauth/authorize", false);
        expectClaudeAuthorization("https://user@claude.com/cai/oauth/authorize", false);
        System.out.println("Browser address and navigation policy passed.");
    }

    private static void expect(String input, String expected) {
        if (!Objects.equals(expected, WebAddress.fromInput(input))) throw new AssertionError(input);
    }

    private static void expectClaudeAuthorization(String url, boolean expected) {
        if (WebAddress.isClaudeCodeAuthorization(url) != expected) throw new AssertionError(url);
    }
}
