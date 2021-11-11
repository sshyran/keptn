import { ComponentFixture, TestBed } from '@angular/core/testing';

import { KtbChartMatrixComponent } from './ktb-chart-matrix.component';

describe('KtbChartMatrixComponent', () => {
  let component: KtbChartMatrixComponent;
  let fixture: ComponentFixture<KtbChartMatrixComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      declarations: [ KtbChartMatrixComponent ]
    })
    .compileComponents();
  });

  beforeEach(() => {
    fixture = TestBed.createComponent(KtbChartMatrixComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
