import type { Checklist } from '@ai-buddy/client';
import { cleanChecklistName, currentStage, weekLabel } from '../../lib/format';
import { ProgressRing } from './ProgressRing';

type Props = { checklist: Checklist; selected: boolean; onSelect: () => void };

export function ChecklistCard({ checklist, selected, onSelect }: Props) {
  const isCommon = checklist.type === 'common';
  const kind = isCommon ? 'Основной' : 'Профессиональный';
  const stage = currentStage(checklist);
  return (
    <section
      className={`checklist-card${selected ? ' is-selected' : ''}`}
      aria-label={`${isCommon ? 'Общий' : 'Профессиональный'} чек-лист`}
      onClick={onSelect}
    >
      <div className="checklist-card-info">
        <h2>
          <button className="card-title" aria-pressed={selected} onClick={onSelect}>
            {kind} чек-лист: {cleanChecklistName(checklist.name)}
          </button>
        </h2>
        <span className="card-weeks">{stage ? weekLabel(stage) : ''}</span>
      </div>
      <div className="checklist-card-progress">
        <ProgressRing
          percent={checklist.completion_percent}
          label={`Прогресс: ${kind.toLowerCase()} чек-лист`}
          size={selected ? 255 : 64}
        />
      </div>
    </section>
  );
}
