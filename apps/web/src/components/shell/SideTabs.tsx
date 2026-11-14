type Props = { onProfile: () => void; onNotes: () => void };

/** Две выступающие вкладки у правого края: личный кабинет и заметки (иконки из UI-кита). */
export function SideTabs({ onProfile, onNotes }: Props) {
  return (
    <div className="side-tabs">
      <button className="side-tab side-tab-dark" aria-label="Профиль" onClick={onProfile}>
        <img src="/icons/user.svg" alt="" width={33} height={33} />
      </button>
      <button className="side-tab side-tab-pink" aria-label="Заметки" onClick={onNotes}>
        <img src="/icons/notes.svg" alt="" width={23} height={26} />
      </button>
    </div>
  );
}
