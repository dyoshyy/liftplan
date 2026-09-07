import { useState } from 'react';
import { setToken } from '../../storage/local';

const MIN_TOKEN_LENGTH = 32;

export function Setup({ pending, onSaved }: { pending: number; onSaved: () => void }) {
  const [value, setValue] = useState('');
  const [error, setError] = useState('');

  const save = () => {
    const v = value.trim();
    if (v.length < MIN_TOKEN_LENGTH) {
      setError('トークンが短すぎます');
      return;
    }
    setToken(v);
    setValue('');
    setError('');
    onSaved();
  };

  return (
    <div className="card">
      <p className="card-title">最初の設定</p>
      <p className="note mb-3">サーバーの認証トークンを入れてください。この端末にだけ保存します。</p>
      <div className="field">
        <input
          type="password"
          autoComplete="off"
          placeholder="トークン"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </div>
      <button type="button" className="btn mt-3" onClick={save}>
        保存する
      </button>
      {error && <p className="note text-red">{error}</p>}
      {/* 溜まっているものは消えない。ここで言わないと、記録ごと消えたと
          思われる。トークンが変わったのは送り先の話で、記録の話ではない。 */}
      {pending > 0 && (
        <p className="note">未送信の記録が {pending} 件あります。トークンを入れ直せば送られます。</p>
      )}
    </div>
  );
}
