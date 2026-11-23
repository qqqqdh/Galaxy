package dev.galaxy.terminal;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.graphics.Insets;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.system.ErrnoException;
import android.util.Base64;
import android.view.WindowInsets;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.InputMethodManager;
import android.webkit.JavascriptInterface;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.TextView;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.ArrayDeque;
import java.util.concurrent.CountDownLatch;
import org.json.JSONObject;

public class TerminalActivity extends Activity {
    public static final class UbuntuActivity extends TerminalActivity {
        @Override boolean isUbuntu() { return true; }
    }

    private static final String ORIGIN = "https://terminal.local/";
    private static final int CLIPBOARD_LIMIT = 64 * 1024;
    private static final int BACKGROUND = 0xff101319;

    private WebView browser;
    private EditorInput editorInput;
    private int editorMode;
    private AlertDialog ubuntuSetup;
    private volatile PtySession session;
    private final ArrayDeque<CountDownLatch> outputAcks = new ArrayDeque<>();

    boolean isUbuntu() { return false; }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        FrameLayout layout = new FrameLayout(this);
        layout.setBackgroundColor(BACKGROUND);
        browser = new WebView(this) {
            @Override public boolean onKeyPreIme(int code, KeyEvent event) {
                if (editorMode == 1 && code == KeyEvent.KEYCODE_ESCAPE && event.hasNoModifiers()) {
                    if (event.getAction() == KeyEvent.ACTION_DOWN && event.getRepeatCount() == 0) evaluate("editorEscape()");
                    return true;
                }
                return super.onKeyPreIme(code, event);
            }
        };
        browser.setBackgroundColor(BACKGROUND);
        browser.getSettings().setJavaScriptEnabled(true);
        browser.getSettings().setAllowFileAccess(false);
        browser.getSettings().setAllowContentAccess(false);
        browser.addJavascriptInterface(new Bridge(), "Android");
        browser.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return true;
            }

            @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                return asset(request);
            }

            @Override public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
                dispose();
                TextView message = new TextView(TerminalActivity.this);
                message.setText("터미널 화면이 종료되었습니다. 앱을 다시 열어 주세요.");
                setContentView(message);
                return true;
            }
        });
        layout.addView(browser, new FrameLayout.LayoutParams(-1, -1));
        editorInput = new EditorInput(this, new EditorInput.Listener() {
            @Override public void edit(String erased, String inserted) {
                if (editorMode >= 2) evaluate("nativeEditorEdit(" + JSONObject.quote(erased) + "," + JSONObject.quote(inserted) + ")");
            }
            @Override public void preview(String text) {
                evaluate("nativeTerminalDraft(" + JSONObject.quote(text) + ")");
            }
            @Override public boolean terminalKey(KeyEvent event) {
                return browser != null && browser.dispatchKeyEvent(event);
            }
        });
        layout.addView(editorInput, new FrameLayout.LayoutParams(1, 1));
        if (Build.VERSION.SDK_INT >= 35) {
            layout.setOnApplyWindowInsetsListener((view, insets) -> {
                Insets safe = insets.getInsets(WindowInsets.Type.systemBars()
                        | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                view.setPadding(safe.left, safe.top, safe.right, safe.bottom);
                return insets;
            });
        }
        setContentView(layout);
        browser.loadUrl(ORIGIN + "index.html");
    }

    private WebResourceResponse asset(WebResourceRequest request) {
        String url = request.getUrl().toString();
        if (url.startsWith(ORIGIN) && request.getMethod().equals("GET")) {
            String name = url.substring(ORIGIN.length());
            String mime;
            switch (name) {
                case "index.html": mime = "text/html"; break;
                case "terminal.css": case "xterm.css": mime = "text/css"; break;
                case "terminal.js": case "xterm.js": case "addon-fit.js": case "addon-web-links.js": mime = "text/javascript"; break;
                default: return missing();
            }
            try {
                return new WebResourceResponse(mime, "UTF-8", getAssets().open(name));
            } catch (IOException error) {
                return missing();
            }
        }
        return missing();
    }

    private static WebResourceResponse missing() {
        return new WebResourceResponse("text/plain", "UTF-8", 404, "Not Found", null,
                new ByteArrayInputStream(new byte[0]));
    }

    private void startSession(int cols, int rows) {
        startSession(cols, rows, null);
    }

    private void startSession(int cols, int rows, UbuntuSetup.Options setup) {
        if (browser == null || session != null) {
            if (setup != null) setup.clearPassword();
            return;
        }
        try {
            if (isUbuntu() && setup == null && !Ubuntu.configured(this)) {
                if (ubuntuSetup == null || !ubuntuSetup.isShowing()) {
                    ubuntuSetup = UbuntuSetup.show(this, options -> startSession(cols, rows, options));
                }
                return;
            }
            PtySession.Launch launch = isUbuntu()
                    ? () -> Ubuntu.prepare(this, this::showProgress, setup) : this::androidShell;
            session = new PtySession(launch, cols, rows, new PtySession.Listener() {
                private CountDownLatch lastOutput;
                @Override public void started() {
                    runOnUiThread(() -> evaluate("shellStarted()"));
                }

                @Override public void output(byte[] bytes, int length) throws InterruptedException {
                    String base64 = Base64.encodeToString(bytes, 0, length, Base64.NO_WRAP);
                    CountDownLatch ack = new CountDownLatch(1);
                    CountDownLatch oldest;
                    synchronized (outputAcks) {
                        // Allow short redraw bursts while keeping at most four 16 KiB reads in flight.
                        outputAcks.addLast(ack);
                        oldest = outputAcks.size() >= 4 ? outputAcks.peekFirst() : null;
                    }
                    lastOutput = ack;
                    runOnUiThread(() -> evaluate("receiveOutput(" + JSONObject.quote(base64) + ")"));
                    if (oldest != null) oldest.await();
                }

                @Override public void ended(String message) {
                    try {
                        if (lastOutput != null) lastOutput.await();
                    } catch (InterruptedException stopped) {
                        Thread.currentThread().interrupt();
                    }
                    runOnUiThread(() -> {
                        session = null;
                        evaluate("shellEnded(" + JSONObject.quote(message) + ")");
                    });
                }
            });
        } catch (IOException | ErrnoException | LinkageError error) {
            if (setup != null) setup.clearPassword();
            evaluate("shellEnded(" + JSONObject.quote("셸을 열 수 없습니다: " + error.getMessage()) + ")");
        }
    }

    private PtySession.Command androidShell() throws IOException {
        File profile = new File(getNoBackupFilesDir(), "galaxy.mkshrc");
        try (InputStream source = getAssets().open("galaxy.mkshrc")) {
            Files.copy(source, profile.toPath(), StandardCopyOption.REPLACE_EXISTING);
        }
        File home = getFilesDir();
        return new PtySession.Command(home, new String[] { "/system/bin/sh", "-i" },
                "HOME=" + home, "TMPDIR=" + getCacheDir(), "ENV=" + profile,
                "HISTFILE=" + new File(home, ".history"), "PATH=/system/bin:/system/xbin:/vendor/bin",
                "TERM=xterm-256color", "LANG=C.UTF-8", "LC_CTYPE=C.UTF-8", "ANDROID_ROOT=/system",
                "ANDROID_DATA=/data");
    }

    private void showProgress(String message) {
        runOnUiThread(() -> evaluate("shellProgress(" + JSONObject.quote(message) + ")"));
    }

    private void evaluate(String script) {
        if (browser != null) browser.evaluateJavascript(script, null);
    }

    private static boolean validSize(int cols, int rows) {
        return cols > 0 && cols <= 1000 && rows > 0 && rows <= 1000;
    }

    private final class Bridge {
        @JavascriptInterface public void editorMode(int mode) {
            runOnUiThread(() -> {
                if (browser == null || mode < 0 || mode > 3) return;
                boolean wasEditing = editorMode >= 2;
                editorInput.setCommitOnly(mode == 3);
                editorMode = mode;
                if (mode >= 2) {
                    editorInput.setVisibility(View.VISIBLE);
                    if (!editorInput.hasFocus()) editorInput.requestFocus();
                    // A tap in WebView can otherwise transfer the Android IME back to xterm's textarea.
                    browser.setFocusable(false);
                } else if (wasEditing) {
                    editorInput.resetContext();
                    browser.setFocusableInTouchMode(true);
                    editorInput.setVisibility(View.GONE);
                    browser.requestFocus();
                    evaluate("term.focus()");
                }
            });
        }

        @JavascriptInterface public void resetEditorContext() {
            runOnUiThread(() -> { if (editorMode >= 2) editorInput.finishContext(); });
        }
        @JavascriptInterface public boolean isUbuntuSession() { return isUbuntu(); }

        @JavascriptInterface public boolean supportsUbuntu() { return Ubuntu.supported(TerminalActivity.this); }

        @JavascriptInterface public void openUbuntu() {
            if (!isUbuntu() && supportsUbuntu()) runOnUiThread(() -> {
                if (browser != null) startActivity(new Intent(TerminalActivity.this, UbuntuActivity.class));
            });
        }

        @JavascriptInterface public void closeUbuntu() {
            if (isUbuntu()) runOnUiThread(() -> finish());
        }

        @JavascriptInterface public void openBrowser() {
            runOnUiThread(() -> {
                if (browser != null) startActivity(new Intent(TerminalActivity.this, BrowserActivity.class));
            });
        }

        @JavascriptInterface public void openLink(String url) {
            runOnUiThread(() -> {
                if (browser == null) return;
                if (!WebAddress.allowed(url)) {
                    evaluate("notice('HTTPS 주소 또는 기기 내부 HTTP 주소를 열 수 있습니다.')");
                    return;
                }
                if (WebAddress.isClaudeCodeAuthorization(url)) {
                    Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(url));
                    intent.addCategory(Intent.CATEGORY_BROWSABLE);
                    try {
                        startActivity(intent);
                    } catch (ActivityNotFoundException unavailable) {
                        evaluate("notice('Claude Code 로그인 링크를 열 웹 브라우저를 설치해 주세요.')");
                    }
                    return;
                }
                Intent intent = new Intent(TerminalActivity.this, BrowserActivity.class);
                intent.putExtra(BrowserActivity.EXTRA_URL, url);
                startActivity(intent);
            });
        }

        @JavascriptInterface public void start(int cols, int rows) {
            if (validSize(cols, rows)) runOnUiThread(() -> startSession(cols, rows));
        }

        @JavascriptInterface public void resize(int cols, int rows) {
            PtySession current = session;
            if (current != null && validSize(cols, rows)) current.resize(cols, rows);
        }

        @JavascriptInterface public void outputConsumed() {
            CountDownLatch ack;
            synchronized (outputAcks) { ack = outputAcks.pollFirst(); }
            if (ack != null) ack.countDown();
        }

        @JavascriptInterface public boolean send(String data) {
            PtySession current = session;
            if (current == null || data == null || data.length() > CLIPBOARD_LIMIT) return false;
            boolean accepted = current.write(data.getBytes(StandardCharsets.UTF_8));
            if (!accepted) {
                runOnUiThread(() -> evaluate("notice('전송 대기열이 가득 찼습니다.')"));
            }
            return accepted;
        }

        @JavascriptInterface public void keyboard() {
            runOnUiThread(() -> {
                if (browser == null) return;
                View target = editorMode >= 2 ? editorInput : browser;
                target.requestFocus();
                getSystemService(InputMethodManager.class).showSoftInput(target, InputMethodManager.SHOW_IMPLICIT);
            });
        }

        @JavascriptInterface public void copy(String text) {
            if (text == null || text.isEmpty()) return;
            runOnUiThread(() -> {
                if (text.length() > CLIPBOARD_LIMIT) {
                    evaluate("notice('복사할 텍스트가 너무 깁니다.')");
                    return;
                }
                getSystemService(ClipboardManager.class).setPrimaryClip(ClipData.newPlainText("Galaxy Terminal", text));
                if (Build.VERSION.SDK_INT < 33) evaluate("notice('복사했습니다.')");
            });
        }

        @JavascriptInterface public void paste() {
            runOnUiThread(() -> {
                ClipData clip = getSystemService(ClipboardManager.class).getPrimaryClip();
                if (clip == null || clip.getItemCount() == 0) return;
                CharSequence text = clip.getItemAt(0).getText();
                if (text == null || text.length() == 0) return;
                if (text.length() > CLIPBOARD_LIMIT - 16) {
                    evaluate("notice('붙여넣을 텍스트가 너무 깁니다.')");
                    return;
                }
                evaluate("pasteText(" + JSONObject.quote(text.toString()) + ")");
            });
        }
    }

    private void dispose() {
        if (ubuntuSetup != null) {
            ubuntuSetup.dismiss();
            ubuntuSetup = null;
        }
        if (session != null) session.close();
        synchronized (outputAcks) {
            for (CountDownLatch ack : outputAcks) ack.countDown();
            outputAcks.clear();
        }
        if (browser != null) {
            ((FrameLayout) browser.getParent()).removeView(browser);
            browser.removeJavascriptInterface("Android");
            browser.destroy();
            browser = null;
        }
    }

    @Override protected void onDestroy() {
        dispose();
        super.onDestroy();
    }
}
