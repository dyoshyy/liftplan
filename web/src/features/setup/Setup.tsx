import { useState } from 'react';
import { setToken } from '../../storage/local';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { Field, Input } from '../../ui/Field';

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
    <Card title="最初の設定">
      <Note className="mb-3">サーバーの認証トークンを入れてください。この端末にだけ保存します。</Note>
      <Field>
        <Input
          type="password"
          autoComplete="off"
          placeholder="トークン"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </Field>
      <Button className="mt-3" onClick={save}>
        保存する
      </Button>
      {error && <Note className="text-red">{error}</Note>}
      {/* 溜まっているものは消えない。ここで言わないと、記録ごと消えたと
          思われる。トークンが変わったのは送り先の話で、記録の話ではない。 */}
      {pending > 0 && (
        <Note>未送信の記録が {pending} 件あります。トークンを入れ直せば送られます。</Note>
      )}
    </Card>
  );
}
