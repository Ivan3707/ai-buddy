import { PencilIcon, UserIcon } from '../icons';

type Props = { onProfile: () => void; onFeedback: () => void };

/** Две выступающие вкладки у правого края: профиль и «Есть идеи?». */
export function SideTabs({ onProfile, onFeedback }: Props) {
  return (
    <div className="side-tabs">
      <button className="side-tab side-tab-dark" aria-label="Профиль" onClick={onProfile}><UserIcon /></button>
      <button className="side-tab side-tab-pink" aria-label="Обратная связь" onClick={onFeedback}><PencilIcon /></button>
    </div>
  );
}
