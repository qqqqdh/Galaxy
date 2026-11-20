package dev.galaxy.terminal;

import android.content.Context;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.view.KeyEvent;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputConnectionWrapper;
import android.widget.EditText;

/** Android owns the composing span and selection. The terminal displays the edits. */
final class EditorInput extends EditText {
    interface Listener {
        void edit(String erased, String inserted);
        boolean terminalKey(KeyEvent event);
    }

    private boolean resetting;
    private final Listener listener;

    EditorInput(Context context, Listener listener) {
        super(context);
        this.listener = listener;
        setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_MULTI_LINE | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI | EditorInfo.IME_FLAG_NO_FULLSCREEN);
        setContentDescription("IDE 편집 입력");
        setSaveEnabled(false);
        setAlpha(0f);
        setVisibility(GONE);
        addTextChangedListener(new TextWatcher() {
            private String previousTail;

            @Override public void beforeTextChanged(CharSequence s, int start, int count, int after) {
                previousTail = resetting ? "" : s.subSequence(start, s.length()).toString();
            }
            @Override public void onTextChanged(CharSequence s, int start, int before, int count) {
                if (!resetting && hasFocus()) listener.edit(previousTail, s.subSequence(start, s.length()).toString());
            }
            @Override public void afterTextChanged(Editable s) { }
        });
    }

    void resetContext() {
        resetting = true;
        try {
            BaseInputConnection.removeComposingSpans(getText());
            getText().clear();
        } finally {
            resetting = false;
        }
    }

    @Override public InputConnection onCreateInputConnection(EditorInfo attrs) {
        InputConnection target = super.onCreateInputConnection(attrs);
        if (target == null) return null;
        return new InputConnectionWrapper(target, false) {
            @Override public boolean deleteSurroundingText(int before, int after) {
                if (before == 1 && after == 0 && length() == 0 && hasFocus()) {
                    listener.edit(" ", "");
                    return true;
                }
                return super.deleteSurroundingText(before, after);
            }
        };
    }

    @Override public boolean dispatchKeyEvent(KeyEvent event) {
        int key = event.getKeyCode();
        boolean command = event.isCtrlPressed() || event.isAltPressed() || event.isMetaPressed() ||
                (key >= KeyEvent.KEYCODE_DPAD_UP && key <= KeyEvent.KEYCODE_DPAD_RIGHT) ||
                key == KeyEvent.KEYCODE_MOVE_HOME || key == KeyEvent.KEYCODE_MOVE_END ||
                key == KeyEvent.KEYCODE_PAGE_UP || key == KeyEvent.KEYCODE_PAGE_DOWN ||
                key == KeyEvent.KEYCODE_TAB || key == KeyEvent.KEYCODE_ESCAPE ||
                key == KeyEvent.KEYCODE_FORWARD_DEL || (key == KeyEvent.KEYCODE_DEL && length() == 0) ||
                (key >= KeyEvent.KEYCODE_F1 && key <= KeyEvent.KEYCODE_F12);
        if (command && !KeyEvent.isModifierKey(key)) {
            if (event.getAction() == KeyEvent.ACTION_DOWN) resetContext();
            listener.terminalKey(event);
            return true;
        }
        return super.dispatchKeyEvent(event);
    }
}
