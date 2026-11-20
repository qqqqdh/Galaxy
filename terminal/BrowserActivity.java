package dev.galaxy.terminal;

import android.app.Activity;
import android.graphics.Bitmap;
import android.graphics.Insets;
import android.os.Build;
import android.os.Bundle;
import android.text.InputType;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowInsets;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;

public final class BrowserActivity extends Activity {
    private WebView page;
    private FrameLayout content;
    private EditText address;
    private ProgressBar progress;
    private TextView error;
    private Button previous, next, reload;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        layout.setBackgroundColor(0xff101319);

        LinearLayout toolbar = new LinearLayout(this);
        toolbar.setGravity(android.view.Gravity.CENTER_VERTICAL);
        toolbar.setPadding(dp(4), 0, dp(4), 0);
        toolbar.setBackgroundColor(0xff191e28);
        toolbar.addView(button("터미널", "터미널로 돌아가기", this::finish));
        previous = button("←", "이전 페이지", () -> page.goBack());
        next = button("→", "다음 페이지", () -> page.goForward());
        reload = button("↻", "새로고침", () -> page.reload());
        toolbar.addView(previous);
        toolbar.addView(next);
        toolbar.addView(reload);

        address = new EditText(this);
        address.setSingleLine(true);
        address.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        address.setImeOptions(EditorInfo.IME_ACTION_GO);
        address.setSelectAllOnFocus(true);
        address.setHint("웹 주소 또는 검색어");
        address.setContentDescription("웹 주소 또는 검색어");
        address.setTextSize(16);
        address.setOnEditorActionListener((view, action, event) -> {
            if (action != EditorInfo.IME_ACTION_GO && (event == null
                    || event.getKeyCode() != KeyEvent.KEYCODE_ENTER || event.getAction() != KeyEvent.ACTION_DOWN)) {
                return false;
            }
            openAddress();
            return true;
        });
        toolbar.addView(address, new LinearLayout.LayoutParams(0, dp(48), 1));
        toolbar.addView(button("이동", "주소 열기", this::openAddress));
        layout.addView(toolbar, new LinearLayout.LayoutParams(-1, -2));

        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        progress.setMax(100);
        progress.setVisibility(View.INVISIBLE);
        layout.addView(progress, new LinearLayout.LayoutParams(-1, dp(3)));

        error = new TextView(this);
        error.setPadding(dp(12), dp(6), dp(12), dp(8));
        error.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        error.setVisibility(View.GONE);
        layout.addView(error, new LinearLayout.LayoutParams(-1, -2));

        content = new FrameLayout(this);
        content.setFocusableInTouchMode(true);
        layout.addView(content, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(layout);
        if (Build.VERSION.SDK_INT >= 35) {
            layout.setOnApplyWindowInsetsListener((view, insets) -> {
                Insets safe = insets.getInsets(WindowInsets.Type.systemBars()
                        | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                view.setPadding(safe.left, safe.top, safe.right, safe.bottom);
                return insets;
            });
        }
        createPage();
        if (state != null) page.restoreState(state);
        content.requestFocus();
        updateControls();
    }

    private void createPage() {
        page = new WebView(this);
        WebSettings settings = page.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSafeBrowsingEnabled(true);
        page.setBackgroundColor(0xff101319);
        page.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                if (!request.isForMainFrame() || WebAddress.allowed(request.getUrl().toString())) return false;
                showError("HTTPS 주소 또는 기기 내부 HTTP 주소를 열 수 있습니다.");
                return true;
            }

            @Override public void onPageStarted(WebView view, String url, Bitmap icon) {
                error.setVisibility(View.GONE);
                updateAddress(url);
            }

            @Override public void doUpdateVisitedHistory(WebView view, String url, boolean reloading) {
                updateAddress(url);
                updateControls();
            }

            @Override public void onPageFinished(WebView view, String url) {
                updateAddress(url);
                updateControls();
            }

            @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError failure) {
                if (request.isForMainFrame()) showError("페이지를 열 수 없습니다. 주소와 연결을 확인하세요.");
            }

            @Override public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail details) {
                destroyPage();
                updateControls();
                showError("브라우저 화면이 종료되었습니다. 주소를 다시 열 수 있습니다.");
                return true;
            }
        });
        page.setWebChromeClient(new WebChromeClient() {
            @Override public void onProgressChanged(WebView view, int percent) {
                progress.setProgress(percent);
                progress.setVisibility(percent == 100 ? View.INVISIBLE : View.VISIBLE);
            }
        });
        content.addView(page, new FrameLayout.LayoutParams(-1, -1));
    }

    private void openAddress() {
        String url = WebAddress.fromInput(address.getText().toString());
        if (url == null) {
            address.setError("검색어, HTTPS 주소 또는 기기 내부 HTTP 주소를 입력하세요.");
            address.requestFocus();
            return;
        }
        address.setError(null);
        address.setText(url);
        getSystemService(InputMethodManager.class).hideSoftInputFromWindow(address.getWindowToken(), 0);
        if (page == null) createPage();
        page.requestFocus();
        page.loadUrl(url);
    }

    private void updateAddress(String url) {
        if (!address.hasFocus() && WebAddress.allowed(url)) address.setText(url);
    }

    private void updateControls() {
        previous.setEnabled(page != null && page.canGoBack());
        next.setEnabled(page != null && page.canGoForward());
        reload.setEnabled(page != null && page.getUrl() != null);
    }

    private void showError(String message) {
        progress.setVisibility(View.INVISIBLE);
        error.setText(message);
        error.setVisibility(View.VISIBLE);
    }

    private Button button(String label, String description, Runnable action) {
        Button button = new Button(this);
        button.setText(label);
        button.setAllCaps(false);
        button.setContentDescription(description);
        button.setMinWidth(dp(42));
        button.setMinimumWidth(dp(42));
        button.setMinimumHeight(dp(48));
        button.setPadding(dp(8), 0, dp(8), 0);
        button.setOnClickListener(view -> action.run());
        return button;
    }

    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }

    private void destroyPage() {
        if (page == null) return;
        content.removeView(page);
        page.destroy();
        page = null;
    }

    @Override public void onBackPressed() {
        if (page != null && page.canGoBack()) page.goBack();
        else super.onBackPressed();
    }

    @Override protected void onSaveInstanceState(Bundle state) {
        super.onSaveInstanceState(state);
        if (page != null) page.saveState(state);
    }

    @Override protected void onDestroy() {
        destroyPage();
        super.onDestroy();
    }
}
