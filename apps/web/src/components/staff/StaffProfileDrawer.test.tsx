import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StaffProfileDrawer } from './StaffProfileDrawer';
import { buddyUser, leadUser } from '../../test/fixtures';

describe('Личный кабинет сотрудника', () => {
  it('показывает роль, почту и подразделение без выдуманных полей', () => {
    render(<StaffProfileDrawer user={{ ...buddyUser, department: 'Technology', team: 'Onboarding' }} open onClose={() => {}} />);
    expect(screen.getByText('Наставник')).toBeInTheDocument();
    expect(screen.getByText('buddy@example.com')).toBeInTheDocument();
    expect(screen.getByText('Technology')).toBeInTheDocument();
    expect(screen.getByText('Onboarding')).toBeInTheDocument();
    expect(screen.queryByText('Возраст:')).not.toBeInTheDocument();
  });

  it('руководитель: роль и возраст, если он задан', () => {
    render(<StaffProfileDrawer user={{ ...leadUser, age_years: 35 }} open onClose={() => {}} />);
    expect(screen.getByText('Руководитель')).toBeInTheDocument();
    expect(screen.getByText('35 лет')).toBeInTheDocument();
  });
});
