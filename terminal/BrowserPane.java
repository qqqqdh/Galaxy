package dev.galaxy.terminal;

import android.app.Activity;
import android.graphics.Bitmap;
import android.os.Bundle;
import android.text.InputType;
import android.view.KeyEvent;
import android.view.View;
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

final class BrowserPane extends LinearLayout {
    private final Activity activity;
    private final Button split;
    private int scrollX, scrollY;
    private boolean restoreScroll;
    private WebView page;
    private FrameLayout content;
    private EditText address;
    private ProgressBar progress;
    private TextView error;
    private Button previous, next, reload;

    BrowserPane(Activity activity, Runnable close, Runnable toggleSplit, Bundle state) {
        super(activity);
        this.activity = activity;
        setOrientation(VERTICAL);
        setBackgroundColor(0xff101319);

        LinearLayout toolbar = new LinearLayout(activity);
        toolbar.setGravity(android.view.Gravity.CENTER_VERTICAL);
        toolbar.setPadding(dp(4), 0, dp(4), 0);
        toolbar.setBackgroundColor(0xff191e28);
        toolbar.addView(button("터미널", "터미널로 돌아가기", close));
        split = button("나란히", "터미널과 브라우저를 나란히 표시", toggleSplit);
        toolbar.addView(split);
        previous = button("←", "이전 페이지", () -> page.goBack());
        next = button("→", "다음 페이지", () -> page.goForward());
        reload = button("↻", "새로고침", () -> page.reload());
        toolbar.addView(previous);
        toolbar.addView(next);
        toolbar.addView(reload);

        address = new EditText(activity);
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
        addView(toolbar, new LinearLayout.LayoutParams(-1, -2));

        progress = new ProgressBar(activity, null, android.R.attr.progressBarStyleHorizontal);
        progress.setMax(100);
        progress.setVisibility(View.INVISIBLE);
        addView(progress, new LinearLayout.LayoutParams(-1, dp(3)));

        error = new TextView(activity);
        error.setPadding(dp(12), dp(6), dp(12), dp(8));
        error.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        error.setVisibility(View.GONE);
        addView(error, new LinearLayout.LayoutParams(-1, -2));

        content = new FrameLayout(activity);
        content.setFocusableInTouchMode(true);
        addView(content, new LinearLayout.LayoutParams(-1, 0, 1));
        createPage();
        if (state != null) {
            scrollX = state.getInt("scrollX");
            scrollY = state.getInt("scrollY");
            restoreScroll = true;
            Bundle history = state.getBundle("history");
            if (history != null) page.restoreState(history);
            updateAddress(page.getUrl());
        }
        content.requestFocus();
        updateControls();
    }

    private void createPage() {
        page = new WebView(activity);
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
                if (restoreScroll) {
                    restoreScroll = false;
                    view.postVisualStateCallback(0, new WebView.VisualStateCallback() {
                        @Override public void onComplete(long id) { view.scrollTo(scrollX, scrollY); }
                    });
                }
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
        navigate(url);
    }

    void navigate(String url) {
        if (!WebAddress.allowed(url)) return;
        address.setError(null);
        address.setText(url);
        activity.getSystemService(InputMethodManager.class).hideSoftInputFromWindow(address.getWindowToken(), 0);
        if (page == null) createPage();
        page.requestFocus();
        if (!url.equals(page.getUrl())) page.loadUrl(url);
    }

    void setSplit(boolean allowed, boolean active) {
        split.setVisibility(allowed ? View.VISIBLE : View.GONE);
        split.setText(active ? "전체" : "나란히");
        split.setContentDescription(active ? "브라우저 전체 화면" : "터미널과 브라우저를 나란히 표시");
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
        Button button = new Button(activity);
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

    boolean goBack() {
        if (page == null || !page.canGoBack()) return false;
        page.goBack();
        return true;
    }

    Bundle save() {
        Bundle state = new Bundle();
        if (page != null) {
            Bundle history = new Bundle();
            page.saveState(history);
            state.putBundle("history", history);
            state.putInt("scrollX", page.getScrollX());
            state.putInt("scrollY", page.getScrollY());
        }
        return state;
    }

    void dispose() { destroyPage(); }
}
