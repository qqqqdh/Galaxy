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
        void preview(String text);
        boolean terminalKey(KeyEvent event);
    }

    private boolean resetting;
    private boolean commitOnly;
    private String committed = "";
    private final Runnable flush = this::flushCommitted;
    private final Listener listener;

    EditorInput(Context context, Listener listener) {
        super(context);
        this.listener = listener;
        setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_MULTI_LINE | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI | EditorInfo.IME_FLAG_NO_FULLSCREEN);
        setContentDescription("터미널 및 IDE 입력");
        setSaveEnabled(false);
        setAlpha(0f);
        setVisibility(GONE);
        addTextChangedListener(new TextWatcher() {
            private String previousTail;

            @Override public void beforeTextChanged(CharSequence s, int start, int count, int after) {
                previousTail = resetting ? "" : s.subSequence(start, s.length()).toString();
            }
            @Override public void onTextChanged(CharSequence s, int start, int before, int count) {
                if (resetting || !hasFocus()) return;
                if (commitOnly) {
                    removeCallbacks(flush);
                    post(flush);
                } else listener.edit(previousTail, s.subSequence(start, s.length()).toString());
            }
            @Override public void afterTextChanged(Editable s) { }
        });
    }

    void setCommitOnly(boolean enabled) {
        if (commitOnly == enabled) return;
        resetContext();
        commitOnly = enabled;
    }

    private void flushCommitted() {
        if (!commitOnly || resetting || !hasFocus()) return;
        Editable value = getText();
        String text = value.toString();
        int composing = BaseInputConnection.getComposingSpanStart(value);
        String stable = composing < 0 ? text : text.substring(0, composing);
        // Samsung may reselect committed text when switching languages; wait for its replacement.
        if (composing < 0 || stable.startsWith(committed)) {
            if (!committed.equals(stable)) listener.edit(committed, stable);
            committed = stable;
        }
        listener.preview(composing < 0 ? "" : text.substring(Math.min(composing, committed.length())));
    }

    void resetContext() {
        removeCallbacks(flush);
        resetting = true;
        try {
            BaseInputConnection.removeComposingSpans(getText());
            getText().clear();
            committed = "";
            if (commitOnly) listener.preview("");
        } finally {
            resetting = false;
        }
    }

    void finishContext() {
        if (commitOnly) {
            BaseInputConnection.removeComposingSpans(getText());
            flushCommitted();
        }
        resetContext();
    }

    @Override public InputConnection onCreateInputConnection(EditorInfo attrs) {
        InputConnection target = super.onCreateInputConnection(attrs);
        if (target == null) return null;
        return new InputConnectionWrapper(target, false) {
            @Override public boolean setComposingText(CharSequence text, int cursor) {
                boolean accepted = super.setComposingText(text, cursor);
                flushCommitted();
                return accepted;
            }
            @Override public boolean commitText(CharSequence text, int cursor) {
                boolean accepted = super.commitText(text, cursor);
                flushCommitted();
                return accepted;
            }
            @Override public boolean finishComposingText() {
                boolean accepted = super.finishComposingText();
                flushCommitted();
                return accepted;
            }
            @Override public boolean deleteSurroundingText(int before, int after) {
                if (before == 1 && after == 0 && length() == 0 && hasFocus()) {
                    listener.edit(" ", "");
                    return true;
                }
                boolean accepted = super.deleteSurroundingText(before, after);
                flushCommitted();
                return accepted;
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
            if (event.getAction() == KeyEvent.ACTION_DOWN) {
                finishContext();
            }
            listener.terminalKey(event);
            return true;
        }
        return super.dispatchKeyEvent(event);
    }
}
