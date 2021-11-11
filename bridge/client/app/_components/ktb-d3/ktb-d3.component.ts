import { Component, OnInit } from '@angular/core';
import * as d3 from 'd3';
// eslint-disable-next-line import/no-extraneous-dependencies
import { BaseType } from 'd3-selection';

type DataPoint = { group: string; variable: string; value: number; index: number };

@Component({
  selector: 'ktb-d3',
  templateUrl: './ktb-d3.component.html',
  styleUrls: ['./ktb-d3.component.scss'],
})
export class KtbD3Component implements OnInit {
  private chartSelector = 'div#myChart';
  private margin = 200;
  private width = 1920 - this.margin * 2;
  private height = 450 - this.margin * 2;
  private selectedElement?: BaseType;

  public ngOnInit(): void {
    this.createSvg();
  }

  private createSvg(): void {
    const svg = d3
      .select(this.chartSelector)
      .append('svg')
      .attr('width', this.width + this.margin * 2)
      .attr('height', this.height + this.margin * 2)
      .append('g')
      .attr('transform', 'translate(' + this.margin + ',' + this.margin + ')');

    this.setData(svg);
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private setData(svg: d3.Selection<SVGGElement, unknown, HTMLElement, any>): void {
    // Labels of row and columns -> unique identifier of the column called 'group' and 'variable'
    const data = this.generateData(20);
    const myGroups = Array.from(new Set(data.map((d) => d.group)));
    const myVars = Array.from(new Set(data.map((d) => d.variable)));
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    const _this = this;

    // Build X scales and axis:
    const x = d3.scaleBand().range([0, this.width]).domain(myGroups).padding(0.05);
    svg
      .append('g')
      .style('font-size', 15)
      .attr('transform', `translate(0, ${this.height})`)
      .call(d3.axisBottom(x).tickSize(0))
      .select('.domain')
      .remove();

    // Build Y scales and axis:
    const y = d3.scaleBand().range([this.height, 0]).domain(myVars).padding(0.05);
    svg.append('g').style('font-size', 15).call(d3.axisLeft(y).tickSize(0)).select('.domain').remove();

    // create a tooltip
    const tooltip = d3
      .select(this.chartSelector)
      .append('div')
      .style('opacity', 0)
      .attr('class', 'tooltip')
      .style('background-color', 'white')
      .style('border', 'solid')
      .style('border-width', '2px')
      .style('border-radius', '5px')
      .style('padding', '5px');

    const click = function <GElement extends BaseType>(this: GElement, event: MouseEvent, d: DataPoint): void {
      if (_this.selectedElement) {
        // remove selection of previous element
        d3.select(_this.selectedElement).style('fill', _this.getColor(d)).style('opacity', 1);
      }

      d3.select(this).style('fill', 'black').style('opacity', 0.5);
      _this.selectedElement = this;
    };

    // Three function that change the tooltip when user hover / move / leave a cell
    const mouseover = function <GElement extends BaseType>(this: GElement, event: MouseEvent, d: DataPoint): void {
      tooltip.style('opacity', 1);
      d3.select(this).style('stroke', 'black').style('opacity', 0.5);
    };
    const mousemove = function (event: MouseEvent, d: DataPoint): void {
      tooltip
        .html('The exact value of<br>this cell is: ' + d.value)
        .style('left', event.x / 2 + 'px')
        .style('top', event.y / 2 + 'px');
    };
    const mouseleave = function <GElement extends BaseType>(this: GElement, event: MouseEvent, d: DataPoint): void {
      tooltip.style('opacity', 0);
      d3.select(this).style('stroke', 'none').style('opacity', 1);
    };

    // add the squares
    svg
      .selectAll()
      .data(data, function (d) {
        return d ? d.group + ':' + d.variable : '';
      })
      .join('rect')
      .attr('x', function (d) {
        return x(d.group) ?? null;
      })
      .attr('y', function (d) {
        return y(d.variable) ?? null;
      })
      .attr('rx', 4)
      .attr('ry', 4)
      .attr('width', x.bandwidth())
      .attr('height', y.bandwidth())
      .style('fill', this.getColor)
      .style('stroke-width', 4)
      .style('stroke', 'none')
      .style('opacity', 1)
      .on('click', click)
      .on('mouseover', mouseover)
      .on('mousemove', mousemove)
      .on('mouseleave', mouseleave);
  }

  private generateData(counter: number): DataPoint[] {
    const categories = ['score', 'response time p95'];
    const data = [];
    for (const category of categories) {
      for (let i = 0; i < counter; ++i) {
        data.push({
          group: `date_${i}`,
          variable: category,
          value: Math.floor(Math.random() * 3),
          index: i,
        });
      }
    }
    return data;
  }

  private getColor(d: DataPoint): string {
    let color: string;
    if (d.value === 0) {
      color = '#dc172a';
    } else if (d.value === 1) {
      color = '#e6be00';
    } else {
      color = '#7dc540';
    }
    return color;
  }
}
