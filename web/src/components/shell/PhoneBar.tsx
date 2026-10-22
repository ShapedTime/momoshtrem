'use client';

import Link from 'next/link';
import { Mark } from '@/components/brand';
import { UserMenu } from './UserMenu';

/**
 * Phone and tablet top row: brand + account. The tab bar carries navigation.
 * v2 screens with a large title or hero replace it with their own header.
 */
export function PhoneBar() {
  return (
    <header className="phone-bar">
      <Link href="/" className="brand rounded-md" aria-label="momoshtrem, For You">
        <Mark size={26} />
        <span>momoshtrem</span>
      </Link>
      <UserMenu />
    </header>
  );
}
