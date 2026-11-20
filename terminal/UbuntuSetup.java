package dev.galaxy.terminal;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.text.InputType;
import android.text.method.PasswordTransformationMethod;
import android.view.View;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.widget.ArrayAdapter;
import android.widget.AutoCompleteTextView;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;
import java.util.function.Consumer;

final class UbuntuSetup {
    static final class Options {
        final String user, fullName, computer, locale, zone;
        final byte[] password;

        Options(String user, String fullName, String computer, String locale, String zone, byte[] password) {
            this.user = user;
            this.fullName = fullName;
            this.computer = computer;
            this.locale = locale;
            this.zone = zone;
            this.password = password;
        }

        void clearPassword() { Arrays.fill(password, (byte)0); }
    }

    static boolean validUser(String user) {
        return user.matches("[a-z_][a-z0-9_-]{0,31}") && !user.equals("root");
    }

    static boolean validComputer(String computer) {
        return computer.matches("[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?");
    }

    static List<String> locales(Context context) throws IOException {
        List<String> names = new ArrayList<>();
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(
                context.getAssets().open("ubuntu-locales.txt"), StandardCharsets.UTF_8))) {
            String line;
            while ((line = reader.readLine()) != null) if (!line.isEmpty()) names.add(line);
        }
        return names;
    }

    static AlertDialog show(Activity activity, Consumer<Options> start) throws IOException {
        List<String> locales = locales(activity);
        String[] zones = TimeZone.getAvailableIDs();
        Arrays.sort(zones);
        LinearLayout fields = new LinearLayout(activity);
        fields.setOrientation(LinearLayout.VERTICAL);
        int padding = Math.round(24 * activity.getResources().getDisplayMetrics().density);
        fields.setPadding(padding, 0, padding, padding);
        TextView help = new TextView(activity);
        help.setText("일반 사용자 계정을 만들고 Ubuntu의 지역을 설정합니다. 이 비밀번호는 sudo 명령에 사용합니다. 초기 설치에는 인터넷이 필요합니다.");
        fields.addView(help);
        EditText name = field(fields, "이름 (선택)", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_CAP_WORDS);
        EditText user = field(fields, "사용자 이름", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        EditText computer = field(fields, "컴퓨터 이름", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        computer.setText("galaxy-ubuntu");
        EditText password = field(fields, "Ubuntu 비밀번호", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        EditText confirmation = field(fields, "비밀번호 확인", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        password.setSaveEnabled(false);
        confirmation.setSaveEnabled(false);
        password.setTransformationMethod(PasswordTransformationMethod.getInstance());
        confirmation.setTransformationMethod(PasswordTransformationMethod.getInstance());
        password.setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_NO);
        confirmation.setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_NO);
        Locale device = Locale.getDefault();
        String localDefault = device.getLanguage() + "_" + device.getCountry() + ".UTF-8";
        AutoCompleteTextView locale = choice(fields, "언어·지역", locales.toArray(new String[0]),
                locales.contains(localDefault) ? localDefault : "en_US.UTF-8");
        String deviceZone = TimeZone.getDefault().getID();
        AutoCompleteTextView zone = choice(fields, "시간대", zones,
                Arrays.binarySearch(zones, deviceZone) >= 0 ? deviceZone : "UTC");
        Button keyboard = new Button(activity);
        keyboard.setText("Android 입력기 선택");
        keyboard.setOnClickListener(view -> activity.getSystemService(InputMethodManager.class).showInputMethodPicker());
        fields.addView(keyboard);

        ScrollView scroll = new ScrollView(activity);
        scroll.addView(fields);
        AlertDialog dialog = new AlertDialog.Builder(activity).setTitle("Ubuntu 초기 설정").setView(scroll)
                .setNegativeButton("취소", (window, button) -> activity.finish())
                .setPositiveButton("Ubuntu 설정", null).create();
        dialog.setOnCancelListener(window -> activity.finish());
        dialog.setOnDismissListener(window -> { password.getText().clear(); confirmation.getText().clear(); });
        dialog.show();
        dialog.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE
                | WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
        dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(button -> {
            String login = user.getText().toString().trim();
            String fullName = name.getText().toString().trim();
            String host = computer.getText().toString().trim();
            String language = locale.getText().toString().trim();
            String timezone = zone.getText().toString().trim();
            String secret = password.getText().toString();
            if (fullName.length() > 80 || fullName.matches("(?s).*[,:\\p{Cntrl}].*")) {
                invalid(name, "이름에는 쉼표, 콜론, 제어 문자를 사용할 수 없습니다.");
            } else if (!validUser(login)) {
                invalid(user, "영문 소문자 또는 _로 시작하고 소문자·숫자·_·-를 사용하세요.");
            } else if (!validComputer(host)) {
                invalid(computer, "영문자·숫자·하이픈으로 이름을 입력하세요.");
            } else if (secret.isEmpty() || secret.matches("(?s).*[\\r\\n\\x00].*")) {
                invalid(password, "비밀번호를 한 줄로 입력하세요.");
            } else if (!secret.contentEquals(confirmation.getText())) {
                invalid(confirmation, "비밀번호가 일치하지 않습니다.");
            } else if (!locales.contains(language)) {
                invalid(locale, "목록에 있는 언어·지역을 입력하세요.");
            } else if (Arrays.binarySearch(zones, timezone) < 0) {
                invalid(zone, "예: Asia/Seoul");
            } else {
                Options options = new Options(login, fullName, host.toLowerCase(Locale.ROOT), language, timezone,
                        secret.getBytes(StandardCharsets.UTF_8));
                dialog.dismiss();
                start.accept(options);
            }
        });
        return dialog;
    }

    private static EditText field(LinearLayout fields, String label, int inputType) {
        EditText input = new EditText(fields.getContext());
        addField(fields, label, input);
        input.setInputType(inputType);
        return input;
    }

    private static AutoCompleteTextView choice(LinearLayout fields, String label, String[] values, String initial) {
        AutoCompleteTextView input = new AutoCompleteTextView(fields.getContext());
        input.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        input.setThreshold(1);
        input.setAdapter(new ArrayAdapter<>(fields.getContext(), android.R.layout.simple_dropdown_item_1line, values));
        input.setText(initial, false);
        input.setOnClickListener(view -> input.showDropDown());
        addField(fields, label, input);
        return input;
    }

    private static void addField(LinearLayout fields, String label, EditText input) {
        input.setId(android.view.View.generateViewId());
        input.setSingleLine(true);
        TextView caption = new TextView(fields.getContext());
        caption.setText(label);
        caption.setLabelFor(input.getId());
        fields.addView(caption);
        fields.addView(input, new LinearLayout.LayoutParams(-1, -2));
    }

    private static void invalid(EditText field, String message) {
        field.setError(message);
        field.requestFocus();
    }
}
