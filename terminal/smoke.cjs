const assert = require('node:assert/strict');
const path = require('node:path');
const cache = path.resolve(__dirname, '../.build-cache');
const { chromium } = require(path.join(cache, 'vscode/node_modules/playwright'));

async function main() {
    const browser = await chromium.launch({ channel: process.env.TERMINAL_TEST_BROWSER || 'msedge' });
    try {
        const page = await browser.newPage({ viewport: { width: 1100, height: 700 } });
        const errors = [];
        page.on('pageerror', error => errors.push(error.message));
        await page.route('https://terminal.local/*', route => {
            const name = new URL(route.request().url()).pathname.slice(1);
            const types = { '.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript' };
            const contentType = types[path.extname(name)];
            if (!contentType || !/^[\w.-]+$/.test(name)) return route.fulfill({ status: 404 });
            const scripts = { 'xterm.js': 'xterm/lib/xterm.js', 'xterm.css': 'xterm/css/xterm.css',
                'addon-fit.js': 'addon-fit/lib/addon-fit.js', 'addon-web-links.js': 'addon-web-links/lib/addon-web-links.js' };
            const file = scripts[name] ? path.join(__dirname, 'node_modules/@xterm', scripts[name]) : path.join(__dirname, 'ui', name);
            return route.fulfill({ path: file, contentType });
        });
        await page.addInitScript(() => {
            const calls = window.calls = { starts: 0, sizes: [], input: [], copied: [], keyboard: 0, acks: 0, ubuntu: 0, back: 0, browser: 0, links: [] };
            window.Android = {
                isUbuntuSession() { return window.name === 'ubuntu'; },
                supportsUbuntu() { return true; },
                openUbuntu() { calls.ubuntu++; },
                closeUbuntu() { calls.back++; },
                openBrowser() { calls.browser++; },
                openLink(url) { calls.links.push(url); },
                start(cols, rows) {
                    calls.starts++;
                    calls.sizes.push([cols, rows]);
                    if (this.isUbuntuSession()) shellProgress('Ubuntu 다운로드 0%');
                    else shellStarted();
                },
                resize(cols, rows) { calls.sizes.push([cols, rows]); },
                send(data) { calls.input.push(data); return true; },
                copy(text) { calls.copied.push(text); },
                paste() { pasteText('붙여넣기'); },
                keyboard() { calls.keyboard++; },
                outputConsumed() { calls.acks++; }
            };
        });
        await page.goto('https://terminal.local/index.html');
        await page.getByText('실행 중').waitFor({ timeout: 5000 });
        await page.getByRole('button', { name: '키보드' }).click();
        assert.equal(await page.evaluate(() => calls.keyboard), 1);

        await page.evaluate(() => {
            window.navigationEvents = [];
            host.addEventListener('keydown', event => {
                if (event.key === 'Tab' || event.key.startsWith('Arrow')) {
                    setTimeout(() => navigationEvents.push(event.defaultPrevented), 0);
                }
            }, true);
        });
        for (const [key, data] of [
            ['Tab', '\t'], ['Shift+Tab', '\x1b[Z'],
            ['ArrowUp', '\x1b[A'], ['ArrowDown', '\x1b[B'],
            ['ArrowRight', '\x1b[C'], ['ArrowLeft', '\x1b[D']
        ]) {
            const count = await page.evaluate(() => navigationEvents.length);
            await page.keyboard.press(key);
            await page.waitForFunction(previous => navigationEvents.length > previous, count);
            assert.equal(await page.evaluate(() => calls.input.at(-1)), data);
            assert.equal(await page.evaluate(() => navigationEvents.at(-1)), true, `${key} must prevent browser navigation`);
            assert(await page.evaluate(() => document.activeElement === term.textarea), `${key} must keep input focused`);
            await page.keyboard.press('x');
            assert.equal(await page.evaluate(() => calls.input.at(-1)), 'x');
        }
        await page.evaluate(() => new Promise(done => term.write('\x1b[?1h', done)));
        await page.keyboard.press('ArrowUp');
        assert.equal(await page.evaluate(() => calls.input.at(-1)), '\x1bOA');
        await page.evaluate(() => new Promise(done => term.write('\x1b[?1l', done)));
        await page.evaluate(() => { calls.input.length = 0; term.textarea.value = ''; });

        const cdp = await page.context().newCDPSession(page);
        const compose = text => cdp.send('Input.imeSetComposition', { text, selectionStart: text.length, selectionEnd: text.length });
        async function rapidLanguageSwitches() {
            await page.evaluate(async () => {
                calls.input.length = 0;
                term.textarea.value = '';
                for (let i = 0; i < 8; i++) {
                    const input = term.textarea;
                    const prefix = input.value;
                    input.setSelectionRange(prefix.length, prefix.length);
                    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Process', keyCode: 229, bubbles: true }));
                    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
                    for (const text of ['ㅎ', '하', '한']) {
                        input.dispatchEvent(new CompositionEvent('compositionupdate', { data: text, bubbles: true }));
                        input.value = prefix + text;
                        input.setSelectionRange(input.value.length, input.value.length);
                        input.dispatchEvent(new InputEvent('input', { data: text, inputType: 'insertCompositionText', isComposing: true, bubbles: true }));
                    }
                    input.dispatchEvent(new CompositionEvent('compositionend', { data: '한', bubbles: true }));
                    await new Promise(done => setTimeout(done, 10));
                    // Samsung can notify the committed edit again after another Process key.
                    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Process', keyCode: 229, bubbles: true }));
                    input.dispatchEvent(new InputEvent('input', { data: '한', inputType: 'insertText', bubbles: true }));
                    await new Promise(done => setTimeout(done, 10));
                }
            });
            assert.equal(await page.evaluate(() => calls.input.join('')), '한'.repeat(8), 'Delayed IME commit notifications must not duplicate text');
            await page.keyboard.type('aa');
            assert.equal(await page.evaluate(() => calls.input.join('')), '한'.repeat(8) + 'aa', 'Intentional repeated letters must be preserved');
            await page.evaluate(() => { calls.input.length = 0; term.textarea.value = ''; });
        }
        await page.evaluate(() => new Promise(done => term.write('\x1b]133;B\x07', done)));
        await page.evaluate(async () => {
            const input = term.textarea;
            input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Process', keyCode: 229, bubbles: true }));
            input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
            input.dispatchEvent(new CompositionEvent('compositionupdate', { data: 'ㅎ', bubbles: true }));
            input.value = 'ㅎ';
            input.setSelectionRange(1, 1);
            await new Promise(done => setTimeout(done, 0));
        });
        assert.deepEqual(await page.evaluate(() => calls.input), ['ㅎ'], 'First Android composition must be sent once');
        await page.evaluate(async () => {
            const input = term.textarea;
            input.dispatchEvent(new CompositionEvent('compositionend', { data: 'ㅎ', bubbles: true }));
            await new Promise(done => setTimeout(done, 0));
            input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Process', keyCode: 229, bubbles: true }));
            input.value = 'l';
            input.dispatchEvent(new InputEvent('input', { data: 'l', inputType: 'insertText', bubbles: true }));
            await new Promise(done => setTimeout(done, 0));
        });
        assert.deepEqual(await page.evaluate(() => calls.input), ['ㅎ', 'l'], 'Direct IME text must still reach the shell');
        await page.evaluate(() => { calls.input.length = 0; });
        for (const text of ['ㅎ', '하', '한']) {
            await compose(text);
            assert(await page.evaluate(() => calls.input.at(-1)).then(data => data.endsWith(text)));
        }
        await cdp.send('Input.insertText', { text: '한' });
        await page.waitForFunction(() => !settling);
        assert.deepEqual(await page.evaluate(() => calls.input), ['ㅎ', '\x7f하', '\x7f한']);

        await compose('ㄱ');
        await compose('');
        await page.waitForFunction(() => !settling);
        assert.deepEqual(await page.evaluate(() => calls.input.slice(-2)), ['ㄱ', '\x7f']);
        await compose('한');
        await cdp.send('Input.insertText', { text: '하' });
        await compose('나');
        await cdp.send('Input.insertText', { text: '나' });
        await page.waitForFunction(() => !settling);
        assert.deepEqual(await page.evaluate(() => calls.input.slice(-3)), ['한', '\x7f하', '나']);

        await page.keyboard.press('Enter');
        await compose('ㅎ');
        await compose('한');
        assert.equal(await page.evaluate(() => calls.input.at(-1)), '\r');
        await cdp.send('Input.insertText', { text: '한' });
        await page.waitForFunction(() => calls.input.at(-1) === '한');
        await page.keyboard.press('Backspace');
        await page.getByRole('button', { name: 'Ctrl', exact: true }).click();
        await page.keyboard.press('c');
        await page.getByRole('button', { name: 'Tab', exact: true }).click();
        await page.getByRole('button', { name: '위 화살표' }).click();
        assert.deepEqual(await page.evaluate(() => calls.input.slice(-4)), ['\x7f', '\x03', '\t', '\x1b[A']);

        await page.evaluate(() => {
            calls.input.length = 0;
            for (let i = 0; i < 8; i++) {
                nativeEditorEdit('', '한');
                nativeEditorEdit('한', '한');
                nativeEditorEdit('', 'aa');
            }
        });
        assert.equal(await page.evaluate(() => calls.input.join('')), '한aa'.repeat(8), 'Native Codex commits must preserve repeated letters without replaying unchanged text');

        await page.evaluate(() => new Promise(done => term.write('\x1b[?1049h', done)));
        await rapidLanguageSwitches();
        await page.evaluate(() => { calls.input.length = 0; term.textarea.value = ''; });
        await page.evaluate(() => term.textarea.dispatchEvent(new KeyboardEvent('keydown', {
            key: 'Process', keyCode: 229, bubbles: true
        })));
        for (const text of ['ㅎ', '하', '한']) {
            await compose(text);
            assert.equal(await page.locator('.ide-draft:visible').innerText(), text);
            assert.deepEqual(await page.evaluate(() => calls.input), []);
        }
        await cdp.send('Input.insertText', { text: '한' });
        await page.keyboard.type('abc');
        await compose('글');
        await cdp.send('Input.insertText', { text: '글' });
        await page.waitForFunction(() => !settling);
        await compose('ㄱ');
        await compose('');
        await page.waitForFunction(() => !settling);
        await page.keyboard.type(' aa');
        assert.deepEqual(await page.evaluate(() => calls.input), ['한', 'a', 'b', 'c', '글', ' ', 'a', 'a']);
        assert(await page.locator('.ide-draft').isHidden());
        await page.evaluate(() => new Promise(done => term.write('\x1b]2;Galaxy IDE · 편집\x07', done)));
        await page.evaluate(() => { calls.input.length = 0; });
        for (const text of ['ㅎ', '하', '한']) await compose(text);
        assert.deepEqual(await page.evaluate(() => calls.input), ['ㅎ', '\x7f하', '\x7f한']);
        assert(await page.locator('.ide-draft').isHidden());
        await cdp.send('Input.insertText', { text: '한' });
        await page.keyboard.type('abc');
        await compose('글');
        await cdp.send('Input.insertText', { text: '글' });
        assert.deepEqual(await page.evaluate(() => calls.input), ['ㅎ', '\x7f하', '\x7f한', 'a', 'b', 'c', '글']);
        await page.evaluate(() => {
            const input = term.textarea;
            input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
            input.dispatchEvent(new CompositionEvent('compositionupdate', { data: '한abc글', bubbles: true }));
            input.dispatchEvent(new InputEvent('input', { data: '한abc글', inputType: 'insertCompositionText', bubbles: true }));
            input.dispatchEvent(new CompositionEvent('compositionend', { data: '한abc글', bubbles: true }));
        });
        assert.equal(await page.evaluate(() => calls.input.length), 7, 'Recommitting the same editable range adds no text');
        await page.evaluate(() => {
            calls.input.length = 0;
            for (let i = 0; i < 3; i++) {
                if (i) nativeEditorEdit('', '\n');
                for (const [before, after] of [['','ㅎ'], ['ㅎ','하'], ['하','한'], ['','abc'], ['','ㄱ'],
                    ['ㄱ','그'], ['그','글'], ['글','글'], ['',' ']]) nativeEditorEdit(before, after);
            }
        });
        const edited = [];
        for (const ch of await page.evaluate(() => calls.input.join(''))) {
            if (ch === '\x7f') edited.pop();
            else edited.push(ch);
        }
        assert.equal(edited.join(''), Array(3).fill('한abc글 ').join('\r'));
        await page.evaluate(() => new Promise(done => term.write('\x1b[6;2H\x1b]2;Galaxy IDE · 터미널 12,3\x07', done)));
        await page.evaluate(() => nativeTerminalDraft('한'));
        assert(await page.evaluate(() => Math.abs(parseFloat(draftView.style.left) - 12 * 100 / term.cols) < 0.001 &&
            Math.abs(parseFloat(draftView.style.top) - 3 * 100 / term.rows) < 0.001));
        await page.evaluate(() => new Promise(done => term.write('\x1b]2;Galaxy IDE · 터미널\x07', done)));
        assert(await page.evaluate(() => Math.abs(parseFloat(draftView.style.left) - 100 / term.cols) < 0.001 &&
            Math.abs(parseFloat(draftView.style.top) - 5 * 100 / term.rows) < 0.001));
        await page.evaluate(() => nativeTerminalDraft(''));
        await page.evaluate(() => new Promise(done => term.write('\x1b]2;Galaxy IDE\x07', done)));
        await page.keyboard.press('Escape');
        assert.equal(await page.evaluate(() => calls.input.at(-1)), '\x1b');
        await page.locator('[data-key="Escape"]').click();
        assert.equal(await page.evaluate(() => calls.input.slice(-2).join('')), '\x1b\x1b');
        await page.evaluate(() => new Promise(done => term.write('\x1b[?1049l', done)));

        const korean = Buffer.from('\x1b[32m한글\x1b[0m');
        for (const bytes of [korean.subarray(0, 6), korean.subarray(6)]) {
            await page.evaluate(data => receiveOutput(data), bytes.toString('base64'));
        }
        await page.waitForFunction(() => calls.acks === 2);
        const cells = await page.evaluate(() => {
            const line = term.buffer.active.getLine(0);
            return [line.translateToString(true), line.getCell(0).getWidth(), line.getCell(2).getWidth()];
        });
        assert.deepEqual(cells, ['한글', 2, 2]);

        const screen = await page.locator('.xterm-screen').boundingBox();
        const [cellWidth, cellHeight] = await page.evaluate(() => [term.cols, term.rows])
            .then(([cols, rows]) => [screen.width / cols, screen.height / rows]);
        await page.mouse.move(screen.x + 1, screen.y + cellHeight / 2);
        await page.mouse.down();
        await page.mouse.move(screen.x + 4.6 * cellWidth, screen.y + cellHeight / 2, { steps: 12 });
        await page.mouse.up();
        await page.mouse.click(screen.x + 5 * cellWidth, screen.y + cellHeight / 2, { button: 'right' });
        await page.locator('#copy').click({ timeout: 3000 });
        assert.equal(await page.evaluate(() => calls.copied[0].trim()), '한글');
        await page.keyboard.press('Control+Shift+V');
        assert.equal(await page.evaluate(() => calls.input.at(-1)), '붙여넣기');

        const longURL = 'https://example.com/login?state=' + 'x'.repeat(220);
        await page.evaluate(url => new Promise(done => term.write('\x1b[2J\x1b[H' + url, done)), longURL);
        async function clickLink(column, row) {
            await page.mouse.move(screen.x + (column + 0.5) * cellWidth, screen.y + (row + 0.5) * cellHeight);
            await page.waitForFunction(() => host.querySelector('.xterm-cursor-pointer'), { }, { timeout: 5000 });
            await page.mouse.down();
            await page.mouse.up();
        }
        await clickLink(4, 1);
        assert.equal(await page.evaluate(() => calls.links.at(-1)), longURL, 'Wrapped login URL must open in the internal browser');
        await page.evaluate(() => new Promise(done => term.write('\x1b[2J\x1b[H\x1b]8;;https://example.com/auth\x07로그인\x1b]8;;\x07', done)));
        await page.mouse.move(screen.x + screen.width - 5, screen.y + screen.height - 5);
        await clickLink(1, 0);
        assert.deepEqual(await page.evaluate(() => calls.links), [longURL, 'https://example.com/auth'], 'OSC 8 links must use the same internal browser');
        await page.evaluate(() => new Promise(done => term.write('\x1b[2J\x1b[Hhttp://localhost:8080/test', done)));
        await page.mouse.move(screen.x + screen.width - 5, screen.y + screen.height - 5);
        await clickLink(5, 0);
        assert.equal(await page.evaluate(() => calls.links.at(-1)), 'http://localhost:8080/test');

        const lines = Array.from({ length: 100 }, (_, i) => `\r\n줄 ${i}`).join('');
        await page.evaluate(data => receiveOutput(data), Buffer.from(lines).toString('base64'));
        await page.waitForFunction(() => calls.acks === 3 && term.buffer.active.baseY > 0);
        const before = await page.evaluate(() => term.buffer.active.viewportY);
        await page.mouse.move(screen.x + 40, screen.y + 45);
        await page.mouse.wheel(0, -121);
        await page.waitForFunction(previous => term.buffer.active.viewportY < previous, before);
        const originalSize = await page.evaluate(() => [term.cols, term.rows]);
        await page.setViewportSize({ width: 360, height: 640 });
        await page.waitForFunction(([cols, rows]) => term.cols < cols && term.rows < rows, originalSize);
        const lastSize = await page.evaluate(() => calls.sizes.at(-1));
        assert(lastSize[0] < originalSize[0] && lastSize[1] < originalSize[1]);
        await page.evaluate(() => shellEnded('셸이 종료되었습니다.'));
        await page.getByRole('button', { name: '새 셸' }).click();
        await page.getByText('실행 중').waitFor();
        await page.getByRole('button', { name: 'Ubuntu', exact: true }).click();
        assert.equal(await page.evaluate(() => calls.ubuntu), 1);
        await page.getByRole('button', { name: '브라우저' }).click();
        assert.equal(await page.evaluate(() => calls.browser), 1);

        await page.evaluate(() => { window.name = 'ubuntu'; });
        await page.reload();
        await page.getByRole('heading', { name: 'Ubuntu 24.04' }).waitFor();
        await page.getByText('Ubuntu 다운로드 0%').waitFor();
        await page.evaluate(() => shellEnded('셸 오류: 연결이 끊겼습니다.'));
        await page.getByRole('button', { name: '다시 시도' }).click();
        await page.getByText('Ubuntu 다운로드 0%').waitFor();
        assert.equal(await page.evaluate(() => calls.starts), 2);
        await page.evaluate(() => shellProgress('Ubuntu 설치 중'));
        await page.getByText('Ubuntu 설치 중').waitFor();
        await page.evaluate(() => shellStarted());
        await page.getByText('실행 중').waitFor();
        await page.getByText('Ubuntu 설치 중').waitFor({ state: 'hidden' });
        await page.getByRole('button', { name: 'Android 셸로 돌아가기' }).click();
        assert.equal(await page.evaluate(() => calls.back), 1);
        assert.deepEqual(errors, []);
        console.log('Passed IME language switching, terminal links, trackpad controls, resize, restart and Ubuntu install/return UI.');
    } finally {
        await browser.close();
    }
}

main().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
