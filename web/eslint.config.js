import js from '@eslint/js';
import globals from 'globals';
import prettier from 'eslint-config-prettier';
import reactHooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist', 'dev-dist', 'node_modules', '.wrangler'] },
  js.configs.recommended,
  tseslint.configs.recommended,
  {
    // React Compiler 向けの新しい規則（purity / refs / set-state-in-effect 等）は
    // 挙動を変える書き換えを要求するので入れない。依存配列の漏れと
    // フックの呼び方だけを見る。
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'error',
      // `const { [id]: _, ...rest } = obj` で「これを除く」を書く型。
      '@typescript-eslint/no-unused-vars': ['error', { ignoreRestSiblings: true }],
    },
  },
  {
    // scripts/*.mjs は Node で動く画面検査。src は型検査（tsc）が見るので、
    // ここでは tsc に任せられない誤り（依存配列の漏れ等）に絞る。
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
  },
  // 整形は prettier の仕事。規則が競合しないよう、最後に置いて整形系を切る。
  prettier,
);
