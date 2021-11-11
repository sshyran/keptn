import { ComponentFixture, TestBed } from '@angular/core/testing';

import { KtbD3Component } from './ktb-d3.component';

describe('KtbD3Component', () => {
  let component: KtbD3Component;
  let fixture: ComponentFixture<KtbD3Component>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      declarations: [ KtbD3Component ]
    })
    .compileComponents();
  });

  beforeEach(() => {
    fixture = TestBed.createComponent(KtbD3Component);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
