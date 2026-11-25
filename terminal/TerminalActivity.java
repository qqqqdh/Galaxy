package dev.galaxy.terminal;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.res.Configuration;
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
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.TextView;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import org.json.JSONObject;

public class TerminalActivity extends Activity {
    public static final class UbuntuActivity extends TerminalActivity {
        @Override boolean isUbuntu() { return true; }
    }

    private static final String ORIGIN = "https://terminal.local/";
    private static final int CLIPBOARD_LIMIT = 64 * 1024;
    private static final int BACKGROUND = 0xff101319;

    private final List<TerminalPane> tabs = new ArrayList<>();
    private TerminalPane active;
    private LinearLayout layout, workArea, terminalArea, tabButtons;
    private FrameLayout tabContent;
    private HorizontalScrollView tabStrip;
    private FileTransfer fileTransfer;
    private BrowserPane preview;
    private Bundle previewState;
    private boolean previewVisible, split;
    private int nextTab = 1;

    boolean isUbuntu() { return false; }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        if (state != null) previewState = state.getBundle("galaxy.preview");
        if (isUbuntu()) fileTransfer = new FileTransfer(this, this::showNotice, state);

        layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        layout.setBackgroundColor(BACKGROUND);
        workArea = new LinearLayout(this);
        layout.addView(workArea, new LinearLayout.LayoutParams(-1, -1));
        terminalArea = new LinearLayout(this);
        terminalArea.setOrientation(LinearLayout.VERTICAL);
        workArea.addView(terminalArea, new LinearLayout.LayoutParams(-1, -1));
        if (isUbuntu()) {
            tabStrip = new HorizontalScrollView(this);
            tabStrip.setBackgroundColor(0xff191e28);
            tabStrip.setHorizontalScrollBarEnabled(false);
            tabButtons = new LinearLayout(this);
            tabStrip.addView(tabButtons);
            terminalArea.addView(tabStrip, new LinearLayout.LayoutParams(-1, -2));
        }
        tabContent = new FrameLayout(this);
        terminalArea.addView(tabContent, new LinearLayout.LayoutParams(-1, 0, 1));
        if (Build.VERSION.SDK_INT >= 35) {
            layout.setOnApplyWindowInsetsListener((view, insets) -> {
                Insets safe = insets.getInsets(WindowInsets.Type.systemBars()
                        | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                view.setPadding(safe.left, safe.top, safe.right, safe.bottom);
                return insets;
            });
        }
        setContentView(layout);
        newTab();
        if (state != null && state.getBoolean("galaxy.preview.visible")) {
            split = state.getBoolean("galaxy.preview.split");
            showPreview(null);
        }
    }

    private void newTab() {
        if (isUbuntu() && active != null && !Ubuntu.configured(this)) {
            showNotice("Ubuntu 초기 설정을 먼저 완료하세요.");
            return;
        }
        TerminalPane tab = new TerminalPane("셸 " + nextTab++);
        tabs.add(tab);
        tabContent.addView(tab, new FrameLayout.LayoutParams(-1, -1));
        selectTab(tab);
    }

    private void selectTab(TerminalPane tab) {
        if (active != null && active != tab) {
            active.finishInput();
            active.setVisibility(View.INVISIBLE);
        }
        active = tab;
        active.setVisibility(View.VISIBLE);
        active.focusInput();
        updateTabs();
    }

    private Button tabButton(String label, String description, Runnable action) {
        Button button = new Button(this, null, android.R.attr.borderlessButtonStyle);
        button.setText(label);
        button.setAllCaps(false);
        button.setContentDescription(description);
        button.setMinimumHeight(Math.round(40 * getResources().getDisplayMetrics().density));
        button.setMinHeight(0);
        button.setOnClickListener(view -> action.run());
        return button;
    }

    private void updateTabs() {
        if (tabButtons == null) return;
        tabButtons.removeAllViews();
        Button current = null;
        for (TerminalPane tab : tabs) {
            Button button = tabButton(tab.name, tab.name, () -> selectTab(tab));
            button.setSelected(tab == active);
            if (tab == active) {
                current = button;
                button.setTextColor(0xff9eddd1);
            }
            button.setOnLongClickListener(view -> {
                EditText name = new EditText(this);
                name.setSingleLine(true);
                name.setText(tab.name);
                new AlertDialog.Builder(this).setTitle("셸 이름").setView(name)
                        .setNegativeButton("취소", null).setPositiveButton("저장", (dialog, which) -> {
                            String value = name.getText().toString().trim();
                            if (!value.isEmpty()) {
                                tab.name = value;
                                updateTabs();
                            }
                        }).show();
                return true;
            });
            tabButtons.addView(button);
        }
        tabButtons.addView(tabButton("+", "새 Ubuntu 셸", this::newTab));
        if (tabs.size() > 1) {
            tabButtons.addView(tabButton("×", "현재 셸 닫기", () -> requestTabClose(active)));
        }
        if (current != null) {
            Button selected = current;
            tabStrip.post(() -> {
                if (selected.getLeft() < tabStrip.getScrollX() ||
                        selected.getRight() > tabStrip.getScrollX() + tabStrip.getWidth()) {
                    tabStrip.smoothScrollTo(selected.getLeft(), 0);
                }
            });
        }
    }

    private void requestTabClose(TerminalPane tab) {
        new AlertDialog.Builder(this).setMessage(tab.name + "에서 실행 중인 프로그램을 종료할까요?")
                .setNegativeButton("취소", null).setPositiveButton("셸 닫기", (dialog, which) -> {
                    int index = tabs.indexOf(tab);
                    if (index < 0 || tabs.size() == 1) return;
                    tab.dispose();
                    tabContent.removeView(tab);
                    tabs.remove(index);
                    if (active == tab) {
                        active = null;
                        selectTab(tabs.get(Math.min(index, tabs.size() - 1)));
                    } else updateTabs();
                }).show();
    }

    private void requestUbuntuExit() {
        if (tabs.size() < 2) {
            finish();
            return;
        }
        new AlertDialog.Builder(this).setMessage("열린 Ubuntu 셸을 모두 종료할까요?")
                .setNegativeButton("취소", null).setPositiveButton("Ubuntu 닫기", (dialog, which) -> finish()).show();
    }

    private boolean canSplit() {
        Configuration config = getResources().getConfiguration();
        return config.orientation == Configuration.ORIENTATION_LANDSCAPE && config.screenWidthDp >= 680;
    }

    private void showPreview(String url) {
        active.finishInput();
        if (preview == null) {
            preview = new BrowserPane(this, this::hidePreview, () -> {
                split = !split;
                placePanes();
            }, previewState);
            workArea.addView(preview);
            previewState = null;
        }
        previewVisible = true;
        placePanes();
        if (url != null) preview.navigate(url);
        else if (!split) preview.requestFocus();
    }

    private void hidePreview() {
        previewVisible = false;
        placePanes();
        active.focusInput();
    }

    private void placePanes() {
        boolean sideBySide = previewVisible && split && canSplit();
        terminalArea.setVisibility(previewVisible && !sideBySide ? View.GONE : View.VISIBLE);
        terminalArea.setLayoutParams(sideBySide ? new LinearLayout.LayoutParams(0, -1, 1)
                : new LinearLayout.LayoutParams(-1, -1));
        if (preview != null) {
            preview.setVisibility(previewVisible ? View.VISIBLE : View.GONE);
            preview.setLayoutParams(sideBySide ? new LinearLayout.LayoutParams(0, -1, 1)
                    : new LinearLayout.LayoutParams(-1, -1));
            preview.setSplit(canSplit(), sideBySide);
        }
    }

    private void showNotice(String text) {
        if (active != null) active.evaluate("notice(" + JSONObject.quote(text) + ")");
    }

    @Override public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        placePanes();
    }

    @Override public void onBackPressed() {
        if (previewVisible) {
            if (!preview.goBack()) hidePreview();
        } else if (isUbuntu()) {
            requestUbuntuExit();
        } else {
            super.onBackPressed();
        }
    }

    @Override protected void onSaveInstanceState(Bundle state) {
        super.onSaveInstanceState(state);
        if (fileTransfer != null) fileTransfer.save(state);
        if (preview != null) state.putBundle("galaxy.preview", preview.save());
        else if (previewState != null) state.putBundle("galaxy.preview", previewState);
        state.putBoolean("galaxy.preview.visible", previewVisible);
        state.putBoolean("galaxy.preview.split", split);
    }

    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (fileTransfer != null) fileTransfer.result(request, result, data);
    }

    @Override protected void onDestroy() {
        for (TerminalPane tab : tabs) tab.dispose();
        if (preview != null) preview.dispose();
        super.onDestroy();
    }

    private final class TerminalPane extends FrameLayout {
        private String name;
        private WebView browser;
        private EditorInput editorInput;
        private int editorMode;
        private AlertDialog ubuntuSetup;
        private volatile PtySession session;
        private final ArrayDeque<CountDownLatch> outputAcks = new ArrayDeque<>();

        TerminalPane(String name) {
            super(TerminalActivity.this);
            this.name = name;
            browser = new WebView(TerminalActivity.this) {
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
                    addView(message);
                    return true;
                }
            });
            addView(browser, new FrameLayout.LayoutParams(-1, -1));
            editorInput = new EditorInput(TerminalActivity.this, new EditorInput.Listener() {
                @Override public void edit(String erased, String inserted) {
                    if (editorMode >= 2) evaluate("nativeEditorEdit(" + JSONObject.quote(erased) + "," + JSONObject.quote(inserted) + ")");
                }
                @Override public void preview(String text) {
                    evaluate("nativeTerminalDraft(" + JSONObject.quote(text) + ")");
                }
                @Override public void enter() { evaluate("sendTerminalKey('Enter')"); }
                @Override public boolean terminalKey(KeyEvent event) {
                    return browser != null && browser.dispatchKeyEvent(event);
                }
            });
            addView(editorInput, new FrameLayout.LayoutParams(1, 1));
            browser.loadUrl(ORIGIN + "index.html");
        }

        private void finishInput() {
            if (editorMode >= 2) editorInput.finishContext();
            if (browser != null) {
                getSystemService(InputMethodManager.class).hideSoftInputFromWindow(browser.getWindowToken(), 0);
            }
        }

        private void focusInput() {
            if (browser == null) return;
            (editorMode >= 2 ? editorInput : browser).requestFocus();
            evaluate(editorMode >= 2 ? "fit.fit()" : "fit.fit(); term.focus()");
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

        private WebResourceResponse missing() {
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
                if (isUbuntu() && setup == null && !Ubuntu.configured(TerminalActivity.this)) {
                    if (ubuntuSetup == null || !ubuntuSetup.isShowing()) {
                        ubuntuSetup = UbuntuSetup.show(TerminalActivity.this, options -> startSession(cols, rows, options));
                    }
                    return;
                }
                PtySession.Launch launch = isUbuntu()
                        ? () -> Ubuntu.prepare(TerminalActivity.this, this::showProgress, setup) : this::androidShell;
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

        private boolean validSize(int cols, int rows) {
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
                        if (active == TerminalPane.this && isShown() && !editorInput.hasFocus()) editorInput.requestFocus();
                        // A tap in WebView can otherwise transfer the Android IME back to xterm's textarea.
                        browser.setFocusable(false);
                    } else if (wasEditing) {
                        editorInput.resetContext();
                        browser.setFocusableInTouchMode(true);
                        editorInput.setVisibility(View.GONE);
                        if (active == TerminalPane.this && isShown()) {
                            browser.requestFocus();
                            evaluate("term.focus()");
                        }
                    }
                });
            }

            @JavascriptInterface public void resetEditorContext() {
                runOnUiThread(() -> { if (editorMode >= 2) editorInput.finishContext(); });
            }
            @JavascriptInterface public void sendEditorKey(String key) {
                runOnUiThread(() -> {
                    if (editorMode >= 2) editorInput.finishContext();
                    evaluate("sendTerminalKey(" + JSONObject.quote(key) + ")");
                });
            }

            @JavascriptInterface public boolean isUbuntuSession() { return isUbuntu(); }

            @JavascriptInterface public boolean supportsUbuntu() { return Ubuntu.supported(TerminalActivity.this); }

            @JavascriptInterface public void transferFiles() {
                runOnUiThread(() -> {
                    if (browser != null && active == TerminalPane.this && fileTransfer != null
                            && Ubuntu.configured(TerminalActivity.this)) {
                        fileTransfer.show();
                    }
                });
            }

            @JavascriptInterface public void fileRequest(String request) {
                if (request == null || request.length() > 2048) return;
                runOnUiThread(() -> {
                if (browser != null && hasWindowFocus() && isShown() && active == TerminalPane.this && fileTransfer != null) {
                        fileTransfer.request(request);
                    }
                });
            }

            @JavascriptInterface public void openUbuntu() {
                if (!isUbuntu() && supportsUbuntu()) runOnUiThread(() -> {
                    if (browser != null) startActivity(new Intent(TerminalActivity.this, UbuntuActivity.class));
                });
            }

            @JavascriptInterface public void closeUbuntu() {
                if (isUbuntu()) runOnUiThread(() -> requestUbuntuExit());
            }

            @JavascriptInterface public void openBrowser() {
                runOnUiThread(() -> {
                    if (browser != null && active == TerminalPane.this) showPreview(null);
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
                    if (active == TerminalPane.this) showPreview(url);
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

    }
}
